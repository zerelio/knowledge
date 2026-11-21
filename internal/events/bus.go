package events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/zerelio/knowledge/internal/storage"
	"github.com/zerelio/knowledge/pkg/types"
)

// Bus dispatches events to registered webhook subscribers.
type Bus struct {
	store  storage.Backend
	logger *slog.Logger
	client *http.Client
}

// New creates a Bus with the given storage backend and logger.
func New(store storage.Backend, logger *slog.Logger) *Bus {
	return &Bus{
		store:  store,
		logger: logger,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Emit looks up all active subscriptions for the event's tenant and type,
// then fires a goroutine per subscription for non-blocking delivery.
func (b *Bus) Emit(ctx context.Context, event *types.Event) {
	subs, err := b.store.ListWebhooksForEvent(ctx, event.TenantID, event.Type)
	if err != nil {
		b.logger.Error("events: list webhooks for event", "error", err, "event_type", event.Type)
		return
	}
	for _, sub := range subs {
		sub := sub // capture
		go b.deliver(sub, event)
	}
}

// deliver marshals the event as JSON, signs it with HMAC-SHA256, and POSTs to
// the subscriber's URL. It retries up to 3 times with 2s/4s/8s back-off on
// any non-2xx response or transport error.
func (b *Bus) deliver(sub *types.WebhookSubscription, event *types.Event) {
	body, err := json.Marshal(event)
	if err != nil {
		b.logger.Error("events: marshal event", "error", err, "event_id", event.ID)
		return
	}

	sig := computeHMAC([]byte(sub.Secret), body)

	delays := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	for attempt := 0; attempt <= len(delays); attempt++ {
		if attempt > 0 {
			time.Sleep(delays[attempt-1])
		}

		req, err := http.NewRequest(http.MethodPost, sub.URL, bytes.NewReader(body))
		if err != nil {
			b.logger.Error("events: build request", "error", err, "url", sub.URL)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Zerelio-Event", string(event.Type))
		req.Header.Set("X-Zerelio-Delivery", event.ID)
		req.Header.Set("X-Zerelio-Signature", fmt.Sprintf("sha256=%s", sig))

		resp, err := b.client.Do(req)
		if err != nil {
			b.logger.Warn("events: deliver failed (transport)", "attempt", attempt+1, "url", sub.URL, "error", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			b.logger.Debug("events: delivered", "event_id", event.ID, "url", sub.URL, "status", resp.StatusCode)
			return
		}
		b.logger.Warn("events: deliver non-2xx", "attempt", attempt+1, "url", sub.URL, "status", resp.StatusCode)
	}
	b.logger.Error("events: delivery exhausted retries", "event_id", event.ID, "url", sub.URL)
}

// computeHMAC returns the hex-encoded HMAC-SHA256 of body signed with secret.
func computeHMAC(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
