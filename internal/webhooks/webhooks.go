package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Dispatcher struct {
	endpoints []string
	client    *http.Client
}

type Payload struct {
	Event     string            `json:"event"`
	Timestamp time.Time         `json:"timestamp"`
	Data      map[string]string `json:"data"`
}

func NewDispatcher(endpoints []string) *Dispatcher {
	return &Dispatcher{
		endpoints: endpoints,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, event string, data map[string]string) {
	if len(d.endpoints) == 0 {
		return
	}

	payload := Payload{
		Event:     event,
		Timestamp: time.Now(),
		Data:      data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Webhook marshal error: %v", err)
		return
	}

	for _, endpoint := range d.endpoints {
		go func(url string) {
			req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := d.client.Do(req)
			if err != nil {
				log.Printf("Webhook delivery to %s failed: %v", url, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 400 {
				log.Printf("Webhook %s returned status %d", url, resp.StatusCode)
			}
		}(endpoint)
	}
}
