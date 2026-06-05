package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type SSEMessage struct {
	Event string
	Data  interface{}
}

type SSEServer struct {
	clients map[chan SSEMessage]bool
	mu      sync.Mutex
}

func NewSSEServer() *SSEServer {
	return &SSEServer{
		clients: make(map[chan SSEMessage]bool),
	}
}

func (s *SSEServer) HandleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan SSEMessage)
	s.mu.Lock()
	s.clients[clientChan] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, clientChan)
		s.mu.Unlock()
		close(clientChan)
	}()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case msg := <-clientChan:
			dataBytes, err := json.Marshal(msg.Data)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Event, string(dataBytes))
			w.(http.Flusher).Flush()
		case <-time.After(30 * time.Second):
			// Keepalive
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			w.(http.Flusher).Flush()
		}
	}
}

func (s *SSEServer) Broadcast(event string, data interface{}) {
	msg := SSEMessage{Event: event, Data: data}
	s.mu.Lock()
	defer s.mu.Unlock()
	for clientChan := range s.clients {
		select {
		case clientChan <- msg:
		default:
		}
	}
}
