package wsutil

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/klog/v2"
)

const (
	protocolPingInterval = 30 * time.Second
	protocolPongTimeout  = 10 * time.Minute
	protocolWriteTimeout = 5 * time.Second
)

type Message struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type Conn struct {
	*websocket.Conn
	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

type Session struct {
	Context context.Context
	Conn    *Conn
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func Serve(w http.ResponseWriter, r *http.Request, handle func(*Session)) {
	rawConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		klog.Errorf("WebSocket upgrade error: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	conn := &Conn{Conn: rawConn}
	defer func() {
		if err := conn.Close(); err != nil {
			klog.Errorf("WebSocket close error %s: %v", conn.RemoteAddr(), err)
		}
	}()
	conn.startProtocolHeartbeat(ctx)

	handle(&Session{
		Context: ctx,
		Conn:    conn,
	})
}

func (c *Conn) WriteJSON(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.Conn.WriteJSON(v)
}

func (c *Conn) WriteControl(messageType int, data []byte, deadline time.Time) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.Conn.WriteControl(messageType, data, deadline)
}

func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.Conn.Close()
	})
	return c.closeErr
}

func (c *Conn) startProtocolHeartbeat(ctx context.Context) {
	_ = c.SetReadDeadline(time.Now().Add(protocolPongTimeout))
	c.SetPongHandler(func(string) error {
		return c.SetReadDeadline(time.Now().Add(protocolPongTimeout))
	})

	ticker := time.NewTicker(protocolPingInterval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.WriteControl(websocket.PingMessage, nil, time.Now().Add(protocolWriteTimeout)); err != nil {
					klog.V(1).Infof("WebSocket protocol ping failed: %v", err)
					_ = c.Close()
					return
				}
			}
		}
	}()
}

func SendMessage(conn *Conn, msgType, data string) error {
	return conn.WriteJSON(Message{Type: msgType, Data: data})
}

func SendError(conn *Conn, message string) error {
	return SendMessage(conn, "error", message)
}

func SendErrorMessage(conn *Conn, message string) {
	if err := SendError(conn, message); err != nil {
		klog.Errorf("Failed to send error message: %v", err)
	}
}

func (s *Session) SendMessage(msgType, data string) error {
	return SendMessage(s.Conn, msgType, data)
}

func (s *Session) SendErrorMessage(message string) {
	SendErrorMessage(s.Conn, message)
}
