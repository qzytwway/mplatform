package provision

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qzytwway/mplatform/pkg/model"
	"k8s.io/klog/v2"
)

const (
	redactedText       = "******"
	minSecretLength    = 6
	minSecretLineLen   = 16
	logBatchSize       = 100
	logFlushInterval   = time.Second
	logChannelCapacity = 1024
	// maxLogLines caps the rows of one provision across all of its runs.
	maxLogLines = 50000
)

// redactor replaces known secrets in text. Secrets are added as soon as they
// are known, before any output that may contain them is written.
type redactor struct {
	mu      sync.RWMutex
	secrets []string
}

func newRedactor() *redactor {
	return &redactor{}
}

// Add registers secrets. Multi-line values such as private keys are also
// registered line by line.
func (r *redactor) Add(values ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range values {
		v = strings.TrimSpace(v)
		if len(v) < minSecretLength {
			continue
		}
		r.secrets = append(r.secrets, v)
		if !strings.Contains(v, "\n") {
			continue
		}
		for _, line := range strings.Split(v, "\n") {
			if line = strings.TrimSpace(line); len(line) >= minSecretLineLen && !strings.HasPrefix(line, "-----") {
				r.secrets = append(r.secrets, line)
			}
		}
	}
	sort.Slice(r.secrets, func(i, j int) bool { return len(r.secrets[i]) > len(r.secrets[j]) })
}

func (r *redactor) Redact(text string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, secret := range r.secrets {
		if strings.Contains(text, secret) {
			text = strings.ReplaceAll(text, secret, redactedText)
		}
	}
	return text
}

// logSink batches log lines into ProvisionLog rows.
type logSink struct {
	provisionID uint
	redact      *redactor
	ch          chan model.ProvisionLog
	done        chan struct{}
	lines       atomic.Int64
}

func newLogSink(provisionID uint, redact *redactor) *logSink {
	s := &logSink{
		provisionID: provisionID,
		redact:      redact,
		ch:          make(chan model.ProvisionLog, logChannelCapacity),
		done:        make(chan struct{}),
	}
	var existing int64
	if err := model.DB.Model(&model.ProvisionLog{}).Where("provision_id = ?", provisionID).Count(&existing).Error; err != nil {
		klog.Errorf("Failed to count provision log lines for provision %d: %v", provisionID, err)
	}
	s.lines.Store(existing)
	go s.loop()
	return s
}

// write records one line; node may be nil for provision-level messages.
// Past maxLogLines it records a single truncation notice and drops the rest.
func (s *logSink) write(node *model.ProvisionNode, step, text string) {
	switch n := s.lines.Add(1); {
	case n == maxLogLines:
		node, text = nil, fmt.Sprintf("Log truncated: limit of %d lines reached", maxLogLines)
	case n > maxLogLines:
		return
	}
	entry := model.ProvisionLog{
		ProvisionID: s.provisionID,
		Step:        step,
		Text:        truncateLine(s.redact.Redact(text)),
		CreatedAt:   time.Now(),
	}
	if node != nil {
		id := node.ID
		entry.NodeID = &id
		entry.Node = nodeLabel(node)
	}
	s.ch <- entry
}

func (s *logSink) loop() {
	defer close(s.done)
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()
	var batch []model.ProvisionLog
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := model.DB.CreateInBatches(batch, logBatchSize).Error; err != nil {
			klog.Errorf("Failed to write %d provision log lines for provision %d: %v", len(batch), s.provisionID, err)
		}
		batch = nil
	}
	for {
		select {
		case entry, ok := <-s.ch:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= logBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// close flushes pending lines. No write may follow.
func (s *logSink) close() {
	close(s.ch)
	<-s.done
}

func nodeLabel(node *model.ProvisionNode) string {
	if node.Hostname != "" {
		return node.Hostname
	}
	return node.IP
}
