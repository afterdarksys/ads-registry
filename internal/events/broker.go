package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Event is the canonical SSE payload, matching the webhooks.Payload shape.
type Event struct {
	Event     string            `json:"event"`
	Timestamp time.Time         `json:"timestamp"`
	Data      map[string]string `json:"data"`
}

// Broker is an in-process pub/sub hub. Callers Publish events; SSE handlers
// subscribe via Subscribe and drain the returned channel.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{
		subscribers: make(map[chan Event]struct{}),
	}
}

// Publish broadcasts an event to all current subscribers. Non-blocking: slow
// subscribers are skipped to avoid stalling the caller.
func (b *Broker) Publish(event string, data map[string]string) {
	e := Event{
		Event:     event,
		Timestamp: time.Now(),
		Data:      data,
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- e:
		default:
		}
	}
}

func (b *Broker) subscribe() chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broker) unsubscribe(ch chan Event) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	b.mu.Unlock()
	close(ch)
}

// ServeSSE is an http.HandlerFunc that streams all events as SSE. An optional
// ?namespace= query param filters events to a specific namespace.
func (b *Broker) ServeSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	namespace := r.URL.Query().Get("namespace")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable Nginx/Traefik buffering
	w.WriteHeader(http.StatusOK)

	// Send an initial ping so the client knows the connection is live.
	fmt.Fprintf(w, ": ping\n\n")
	flusher.Flush()

	ch := b.subscribe()
	defer b.unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if namespace != "" && ev.Data["namespace"] != namespace {
				continue
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Event, data)
			flusher.Flush()
		}
	}
}

// ServeScanSSE streams scan.complete events for a specific digest only.
func (b *Broker) ServeScanSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Digest comes from the URL — caller is responsible for extracting it.
	digest := r.URL.Query().Get("digest")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, ": ping\n\n")
	flusher.Flush()

	ch := b.subscribe()
	defer b.unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if ev.Event != "scan.complete" {
				continue
			}
			if digest != "" && ev.Data["digest"] != digest {
				continue
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: scan.complete\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}
