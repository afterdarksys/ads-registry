package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	maxAttempts = 5
	baseDelay   = time.Second
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
			Timeout: 10 * time.Second,
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
		go d.deliverWithRetry(endpoint, body)
	}
}

// deliverWithRetry attempts to deliver body to url up to maxAttempts times
// using exponential backoff (1s, 2s, 4s, 8s, 16s). Failed deliveries after
// all attempts are logged as dead-letter entries.
func (d *Dispatcher) deliverWithRetry(url string, body []byte) {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := d.deliver(url, body)
		if err == nil {
			if attempt > 1 {
				log.Printf("Webhook delivered to %s after %d attempt(s)", url, attempt)
			}
			return
		}

		if attempt == maxAttempts {
			log.Printf("Webhook dead-letter: all %d attempts to %s failed: %v", maxAttempts, url, err)
			return
		}

		delay := baseDelay * (1 << (attempt - 1)) // 1s, 2s, 4s, 8s
		log.Printf("Webhook delivery to %s failed (attempt %d/%d), retrying in %s: %v", url, attempt, maxAttempts, delay, err)
		time.Sleep(delay)
	}
}

func (d *Dispatcher) deliver(url string, body []byte) error {
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
