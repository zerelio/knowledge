package events_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zerelio/zerelio/internal/events"
	"github.com/zerelio/zerelio/internal/storage"
	"github.com/zerelio/zerelio/pkg/types"
)

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// computeExpectedHMAC mirrors the signature logic in bus.go.
func computeExpectedHMAC(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// TestHMACSignatureFormat verifies the delivered X-Zerelio-Signature header
// is formatted as sha256=<64-char-hex> and matches the expected HMAC.
func TestHMACSignatureFormat(t *testing.T) {
	type delivery struct {
		sig  string
		body []byte
	}
	delivered := make(chan delivery, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get("X-Zerelio-Signature")
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		select {
		case delivered <- delivery{sig, body}:
		default:
		}
	}))
	defer srv.Close()

	mem := storage.NewMemoryBackend()
	_ = mem.Connect(context.Background())

	secret := "supersecret"
	sub := &types.WebhookSubscription{
		ID:       "wh_sig_test",
		TenantID: "t1",
		URL:      srv.URL,
		Secret:   secret,
		Events:   []string{string(types.EventDiscoveryCompleted)},
		Active:   true,
	}
	_ = mem.CreateWebhook(context.Background(), sub)

	bus := events.New(mem, nopLogger())
	event := &types.Event{
		ID:         "evt_sig",
		Type:       types.EventDiscoveryCompleted,
		TenantID:   "t1",
		OccurredAt: time.Now(),
	}
	bus.Emit(context.Background(), event)

	// Wait for delivery via channel (no data race).
	select {
	case d := <-delivered:
		if !strings.HasPrefix(d.sig, "sha256=") {
			t.Fatalf("expected signature prefixed with sha256=, got %q", d.sig)
		}
		hexPart := strings.TrimPrefix(d.sig, "sha256=")
		if len(hexPart) != 64 {
			t.Fatalf("expected 64-char hex, got %d chars in %q", len(hexPart), hexPart)
		}
		body := d.body
		if len(body) == 0 {
			body, _ = json.Marshal(event)
		}
		expected := computeExpectedHMAC([]byte(secret), body)
		if hexPart != expected {
			t.Fatalf("HMAC mismatch: got %q, want %q", hexPart, expected)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for webhook delivery")
	}
}

// TestEmitNoSubscriptionsNoPanic ensures Emit does not panic when no
// subscriptions match.
func TestEmitNoSubscriptionsNoPanic(t *testing.T) {
	mem := storage.NewMemoryBackend()
	_ = mem.Connect(context.Background())
	bus := events.New(mem, nopLogger())

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Emit panicked: %v", r)
		}
	}()

	bus.Emit(context.Background(), &types.Event{
		ID:         "evt_no_subs",
		Type:       types.EventDiscoveryCompleted,
		TenantID:   "nobody",
		OccurredAt: time.Now(),
	})
	time.Sleep(50 * time.Millisecond)
}

// TestDeliverRetries verifies that the bus retries delivery on non-2xx
// responses and that the third attempt (index 2) succeeds, resulting in
// exactly 3 total HTTP requests to the test server.
func TestDeliverRetries(t *testing.T) {
	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mem := storage.NewMemoryBackend()
	_ = mem.Connect(context.Background())

	sub := &types.WebhookSubscription{
		ID:       "wh_retry",
		TenantID: "t2",
		URL:      srv.URL,
		Secret:   "s3cret",
		Events:   []string{string(types.EventDiscoveryCompleted)},
		Active:   true,
	}
	_ = mem.CreateWebhook(context.Background(), sub)

	bus := events.New(mem, nopLogger())
	bus.Emit(context.Background(), &types.Event{
		ID:         "evt_retry",
		Type:       types.EventDiscoveryCompleted,
		TenantID:   "t2",
		OccurredAt: time.Now(),
	})

	// Wait for all 3 attempts (up to 15s to cover 2s + 4s back-off).
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if requestCount.Load() >= 3 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if got := requestCount.Load(); got != 3 {
		t.Fatalf("expected 3 total requests, got %d", got)
	}
}
