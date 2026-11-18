package wsutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/qzytwway/mplatform/pkg/common"
)

func TestServeSerializesConcurrentMessages(t *testing.T) {
	const messageCount = 20
	serverErrors := make(chan error, messageCount)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Serve(w, r, func(session *Session) {
			var wait sync.WaitGroup
			for i := 0; i < messageCount; i++ {
				wait.Add(1)
				go func(index int) {
					defer wait.Done()
					if err := session.SendMessage("stdout", fmt.Sprintf("message-%d", index)); err != nil {
						serverErrors <- err
					}
				}(i)
			}
			wait.Wait()
		})
	}))
	t.Cleanup(server.Close)

	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, response, err := websocket.DefaultDialer.Dial(websocketURL, nil)
	if response != nil {
		defer func() {
			_ = response.Body.Close()
		}()
	}
	if err != nil {
		t.Fatalf("dialing WebSocket: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	received := make(map[string]bool, messageCount)
	for i := 0; i < messageCount; i++ {
		var message Message
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("reading message %d: %v", i, err)
		}
		if message.Type != "stdout" {
			t.Fatalf("message type = %q, want stdout", message.Type)
		}
		received[message.Data] = true
	}
	if len(received) != messageCount {
		t.Fatalf("received %d unique messages, want %d", len(received), messageCount)
	}
	select {
	case err := <-serverErrors:
		t.Fatalf("server write error: %v", err)
	default:
	}
}

func TestSendErrorUsesErrorMessageType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Serve(w, r, func(session *Session) {
			session.SendErrorMessage("terminal unavailable")
		})
	}))
	t.Cleanup(server.Close)

	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, response, err := websocket.DefaultDialer.Dial(websocketURL, nil)
	if response != nil {
		defer func() {
			_ = response.Body.Close()
		}()
	}
	if err != nil {
		t.Fatalf("dialing WebSocket: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	var message Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("reading error message: %v", err)
	}
	if message.Type != "error" || message.Data != "terminal unavailable" {
		t.Fatalf("error message = %#v", message)
	}
}

func TestServeChecksOrigin(t *testing.T) {
	previous := common.CORSAllowedOrigins
	common.CORSAllowedOrigins = []string{"http://localhost:5173"}
	t.Cleanup(func() { common.CORSAllowedOrigins = previous })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Serve(w, r, func(*Session) {})
	}))
	t.Cleanup(server.Close)
	websocketURL := "ws" + strings.TrimPrefix(server.URL, "http")

	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "no origin (non-browser client)", origin: "", want: true},
		{name: "same host", origin: server.URL, want: true},
		{name: "configured dev origin", origin: "http://localhost:5173", want: true},
		{name: "other site", origin: "https://wiki.example.com", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.origin != "" {
				header.Set("Origin", tt.origin)
			}
			conn, response, err := websocket.DefaultDialer.Dial(websocketURL, header)
			if response != nil {
				_ = response.Body.Close()
			}
			if conn != nil {
				_ = conn.Close()
			}
			if got := err == nil; got != tt.want {
				t.Fatalf("connected = %v (err %v), want %v", got, err, tt.want)
			}
		})
	}
}
