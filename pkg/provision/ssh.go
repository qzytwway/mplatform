package provision

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	sshDialTimeout      = 15 * time.Second
	sshHandshakeTimeout = 30 * time.Second
	sshKeepAlive        = 30 * time.Second
	sshProbeTimeout     = 20 * time.Second
	maxLineLength       = 4096
	maxOutputBytes      = 4 << 20
	maxProbeOutputBytes = 1 << 20
	uploadTimeout       = 5 * time.Minute
	errorTailLines      = 8
	remoteEnv           = "export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C; "
)

const (
	privilegeRoot         = "root"
	privilegeSudo         = "sudo"
	privilegeSudoPassword = "sudo-password"
)

// Executor runs commands as root on one node. Implementations redact
// secrets from everything they return.
type Executor interface {
	// Run streams combined output line by line to out.
	Run(ctx context.Context, cmd string, out func(line string)) error
	// Output returns stdout without logging it, for secrets such as admin.conf.
	Output(ctx context.Context, cmd string) ([]byte, error)
	Upload(ctx context.Context, path string, data []byte, mode uint32) error
	// Privilege reports how root is obtained, or why it cannot be.
	Privilege() (string, error)
	Close() error
}

type sshTarget struct {
	IP         string
	Port       int
	User       string
	AuthType   string
	PrivateKey string
	Password   string
	// KnownHostKey is the recorded key in authorized_keys format; when empty
	// any key is accepted and reported through HostKey.
	KnownHostKey string
}

type sshExecutor struct {
	client       *ssh.Client
	target       sshTarget
	redact       func(string) string
	hostKey      string
	privilege    string
	privilegeErr error
	stop         chan struct{}
	closeOnce    sync.Once
}

// dialSSH connects, authenticates, verifies the host key when one is known
// and detects how to become root.
func dialSSH(ctx context.Context, t sshTarget, redact func(string) string) (*sshExecutor, error) {
	e := &sshExecutor{target: t, redact: redact, stop: make(chan struct{})}
	cfg := &ssh.ClientConfig{
		User:            t.User,
		Auth:            authMethods(t),
		HostKeyCallback: e.checkHostKey,
		Timeout:         sshDialTimeout,
	}
	if t.KnownHostKey != "" {
		if known, _, _, _, err := ssh.ParseAuthorizedKey([]byte(t.KnownHostKey)); err == nil {
			cfg.HostKeyAlgorithms = hostKeyAlgorithms(known.Type())
		}
	}
	addr := net.JoinHostPort(t.IP, strconv.Itoa(t.Port))
	conn, err := (&net.Dialer{Timeout: sshDialTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w", addr, err)
	}
	_ = conn.SetDeadline(time.Now().Add(sshHandshakeTimeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("SSH login to %s as %s failed: %s", addr, t.User, redact(err.Error()))
	}
	_ = conn.SetDeadline(time.Time{})
	e.client = ssh.NewClient(c, chans, reqs)
	go e.keepAlive()
	e.detectPrivilege(ctx)
	return e, nil
}

func authMethods(t sshTarget) []ssh.AuthMethod {
	if t.AuthType == AuthPassword {
		password := t.Password
		return []ssh.AuthMethod{
			ssh.Password(password),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = password
				}
				return answers, nil
			}),
		}
	}
	signer, err := ssh.ParsePrivateKey([]byte(t.PrivateKey))
	if err != nil {
		return nil
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}
}

func hostKeyAlgorithms(keyType string) []string {
	if keyType == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	}
	return []string{keyType}
}

func (e *sshExecutor) checkHostKey(_ string, _ net.Addr, key ssh.PublicKey) error {
	e.hostKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	if e.target.KnownHostKey == "" || e.hostKey == e.target.KnownHostKey {
		return nil
	}
	expected := "a different key"
	if known, _, _, _, err := ssh.ParseAuthorizedKey([]byte(e.target.KnownHostKey)); err == nil {
		expected = ssh.FingerprintSHA256(known)
	}
	return fmt.Errorf("host key of %s changed (expected %s, got %s); refusing to connect", e.target.IP, expected, ssh.FingerprintSHA256(key))
}

// HostKey is the key the server presented, in authorized_keys format.
func (e *sshExecutor) HostKey() string {
	return e.hostKey
}

func (e *sshExecutor) keepAlive() {
	ticker := time.NewTicker(sshKeepAlive)
	defer ticker.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			if _, _, err := e.client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				_ = e.client.Close()
				return
			}
		}
	}
}

func (e *sshExecutor) Close() error {
	var err error
	e.closeOnce.Do(func() {
		close(e.stop)
		err = e.client.Close()
	})
	return err
}

// detectPrivilege checks the exact sudo form later commands use. A login
// password is only ever sent on stdin, never on a command line.
func (e *sshExecutor) detectPrivilege(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, sshProbeTimeout)
	defer cancel()
	stdout := &limitedBuffer{limit: maxProbeOutputBytes}
	if err := e.exec(ctx, "id -u", nil, stdout, io.Discard); err == nil && !stdout.exceeded && strings.TrimSpace(stdout.String()) == "0" {
		e.privilege = privilegeRoot
		return
	}
	tail, err := e.execTail(ctx, "sudo -n true", nil, io.Discard)
	if err == nil {
		e.privilege = privilegeSudo
		return
	}
	if e.target.AuthType == AuthPassword {
		if tail, err = e.execTail(ctx, "sudo -k -S -p '' true", []byte(e.target.Password+"\n"), io.Discard); err == nil {
			e.privilege = privilegeSudoPassword
			return
		}
	}
	e.privilegeErr = fmt.Errorf("user %s is not root and cannot use sudo: %s", e.target.User, defaultString(tail.String(), "sudo is not available"))
}

func (e *sshExecutor) Privilege() (string, error) {
	return e.privilege, e.privilegeErr
}

// wrap turns cmd into a remote command running as root and the stdin it
// needs: the login password for sudo -S, nothing otherwise.
func (e *sshExecutor) wrap(cmd string) (string, []byte, error) {
	script := shellQuote(remoteEnv + cmd)
	switch e.privilege {
	case privilegeRoot:
		return "bash -c " + script, nil, nil
	case privilegeSudo:
		return "sudo -n -- bash -c " + script, nil, nil
	case privilegeSudoPassword:
		return "sudo -k -S -p '' -- bash -c " + script, []byte(e.target.Password + "\n"), nil
	default:
		return "", nil, e.privilegeErr
	}
}

func (e *sshExecutor) Run(ctx context.Context, cmd string, out func(string)) error {
	remote, stdin, err := e.wrap(cmd)
	if err != nil {
		return err
	}
	tail := newLineTail(errorTailLines)
	var mu sync.Mutex
	emit := func(line string) {
		line = truncateLine(e.redact(line))
		mu.Lock()
		defer mu.Unlock()
		tail.add(line)
		if out != nil {
			out(line)
		}
	}
	stdout, stderr := newLineWriter(emit), newLineWriter(emit)
	err = e.exec(ctx, remote, stdin, stdout, stderr)
	stdout.flush()
	stderr.flush()
	return e.commandError(ctx, err, tail)
}

func (e *sshExecutor) Output(ctx context.Context, cmd string) ([]byte, error) {
	remote, stdin, err := e.wrap(cmd)
	if err != nil {
		return nil, err
	}
	stdout := &limitedBuffer{limit: maxOutputBytes}
	tail, err := e.execTail(ctx, remote, stdin, stdout)
	if err = e.commandError(ctx, err, tail); err == nil && stdout.exceeded {
		err = fmt.Errorf("output exceeds %d bytes", maxOutputBytes)
	}
	return stdout.Bytes(), err
}

// Upload writes a root-owned file atomically. With sudo -S the payload is
// first written to a private temp file as the login user, so the password and
// the payload never share stdin.
func (e *sshExecutor) Upload(ctx context.Context, dest string, data []byte, mode uint32) error {
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()
	dir, tmp := path.Dir(dest), dest+".mplatform-tmp"
	place := fmt.Sprintf("umask 022 && mkdir -p %s && umask 077", shellQuote(dir))
	if e.privilege != privilegeSudoPassword {
		remote, _, err := e.wrap(fmt.Sprintf("%s && tee %s >/dev/null && chmod %04o %s && mv -f %s %s",
			place, shellQuote(tmp), mode, shellQuote(tmp), shellQuote(tmp), shellQuote(dest)))
		if err != nil {
			return err
		}
		tail, err := e.execTail(ctx, remote, data, io.Discard)
		return wrapUploadError(dest, e.commandError(ctx, err, tail))
	}
	src := &limitedBuffer{limit: maxProbeOutputBytes}
	tail, err := e.execTail(ctx, `umask 077 && f=$(mktemp /tmp/mplatform.XXXXXXXX) && cat > "$f" && echo "$f"`, data, src)
	if err = e.commandError(ctx, err, tail); err != nil {
		return wrapUploadError(dest, err)
	}
	if src.exceeded {
		return wrapUploadError(dest, fmt.Errorf("unexpected output while staging the file (over %d bytes)", maxProbeOutputBytes))
	}
	staged := shellQuote(strings.TrimSpace(src.String()))
	err = e.Run(ctx, fmt.Sprintf("rc=0; { %s && install -m %04o %s %s; } || rc=$?; rm -f %s; exit $rc",
		place, mode, staged, shellQuote(dest), staged), nil)
	return wrapUploadError(dest, err)
}

func wrapUploadError(dest string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("upload %s: %w", dest, err)
}

// execTail runs remote and keeps the last stderr lines for error messages.
func (e *sshExecutor) execTail(ctx context.Context, remote string, stdin []byte, stdout io.Writer) (*lineTail, error) {
	tail := newLineTail(errorTailLines)
	stderr := newLineWriter(func(line string) { tail.add(truncateLine(e.redact(line))) })
	err := e.exec(ctx, remote, stdin, stdout, stderr)
	stderr.flush()
	return tail, err
}

func (e *sshExecutor) exec(ctx context.Context, remote string, stdin []byte, stdout, stderr io.Writer) error {
	session, err := e.client.NewSession()
	if err != nil {
		return fmt.Errorf("open SSH session: %w", err)
	}
	defer func() { _ = session.Close() }()
	session.Stdin = bytes.NewReader(stdin)
	session.Stdout = stdout
	session.Stderr = stderr
	if err := session.Start(remote); err != nil {
		return fmt.Errorf("start remote command: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		_ = session.Close()
		if err := <-done; err != nil {
			return err
		}
		return ctx.Err()
	}
}

func (e *sshExecutor) commandError(ctx context.Context, err error, tail *lineTail) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return fmt.Errorf("timed out: %s", defaultString(tail.String(), "no output"))
		}
		return ctxErr
	}
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		if t := tail.String(); t != "" {
			return fmt.Errorf("exit status %d: %s", exitErr.ExitStatus(), t)
		}
		return fmt.Errorf("exit status %d", exitErr.ExitStatus())
	}
	return errors.New(e.redact(err.Error()))
}

func truncateLine(line string) string {
	if len(line) <= maxLineLength {
		return line
	}
	return line[:maxLineLength] + " …(truncated)"
}

// lineWriter splits a byte stream into lines for emit.
type lineWriter struct {
	emit func(string)
	buf  []byte
}

func newLineWriter(emit func(string)) *lineWriter {
	return &lineWriter{emit: emit}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(strings.TrimRight(string(w.buf[:i]), "\r"))
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > maxLineLength*2 {
		w.emit(string(w.buf))
		w.buf = nil
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.emit(strings.TrimRight(string(w.buf), "\r"))
		w.buf = nil
	}
}

// lineTail keeps the last lines of output for error messages.
type lineTail struct {
	max   int
	lines []string
}

func newLineTail(max int) *lineTail {
	return &lineTail{max: max}
}

func (t *lineTail) add(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	t.lines = append(t.lines, line)
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
}

func (t *lineTail) String() string {
	return strings.Join(t.lines, "\n")
}

// limitedBuffer keeps at most limit bytes and records whether more arrived.
type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

// Write always reports the full length so the SSH stream keeps draining.
func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if room := b.limit - b.Len(); n > room {
		b.exceeded = true
		p = p[:max(room, 0)]
	}
	b.Buffer.Write(p)
	return n, nil
}
