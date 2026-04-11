package torbox

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/MunifTanjim/stremthru/store"
)

func TestGenerateLinkFailoverDoesNotBlockFallbackKeyOn403(t *testing.T) {
	previousPool := Pool
	t.Cleanup(func() {
		Pool = previousPool
	})

	Pool = NewKeyPool([]string{"primary-key", "fallback-key"})
	Pool.RecordError("primary-key", http.StatusUnauthorized)

	var seenAuth string
	var seenToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenToken = r.URL.Query().Get("token")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"detail":"forbidden","error":"UNKNOWN_ERROR"}`))
	}))
	defer server.Close()

	client := NewStoreClient(&StoreClientConfig{
		HTTPClient: server.Client(),
		UserAgent:  "stremthru-test",
	})

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}

	client.client.BaseURL = baseURL
	client.client.HTTPClient = server.Client()
	client.client.apiKey = "primary-key"

	_, err = client.GenerateLink(&store.GenerateLinkParams{
		Link: LockedFileLink("").Create(123, 456),
	})
	if err == nil {
		t.Fatal("expected GenerateLink error")
	}

	if seenAuth != "Bearer fallback-key" {
		t.Fatalf("expected fallback Authorization header, got %q", seenAuth)
	}

	if seenToken != "fallback-key" {
		t.Fatalf("expected fallback token query, got %q", seenToken)
	}

	if got := Pool.keys[0].health; got != KeyHealthBlocked {
		t.Fatalf("expected primary key to stay blocked, got %q", got)
	}

	if got := Pool.keys[1].health; got != KeyHealthHealthy {
		t.Fatalf("expected fallback key to remain healthy, got %q", got)
	}
}

func TestAutoRecoveryAfterTimeout(t *testing.T) {
	savedTimeout := keyPoolRecoveryTimeout
	t.Cleanup(func() { keyPoolRecoveryTimeout = savedTimeout })
	keyPoolRecoveryTimeout = 50 * time.Millisecond

	pool := NewKeyPool([]string{"key-a", "key-b"})
	pool.RecordError("key-a", 429)

	if pool.keys[0].health != KeyHealthRateLimited {
		t.Fatalf("expected key-a rate_limited, got %q", pool.keys[0].health)
	}

	// Before timeout: should failover to key-b
	got := pool.GetKeyForRequest("key-a")
	if got != "key-b" {
		t.Fatalf("expected failover to key-b before recovery, got %q", got)
	}

	time.Sleep(60 * time.Millisecond)

	// After timeout: key-a should auto-recover
	got = pool.GetKeyForRequest("key-a")
	if got != "key-a" {
		t.Fatalf("expected key-a to auto-recover, got %q", got)
	}

	if pool.keys[0].health != KeyHealthHealthy {
		t.Fatalf("expected key-a healthy after recovery, got %q", pool.keys[0].health)
	}
}

func TestAllKeysUnhealthyFallback(t *testing.T) {
	pool := NewKeyPool([]string{"key-a", "key-b", "key-c"})
	pool.RecordError("key-a", 429)
	pool.RecordError("key-b", 401)
	pool.RecordError("key-c", 403)

	// All unhealthy — should return the incoming key as last resort
	got := pool.GetKeyForRequest("key-a")
	if got != "key-a" {
		t.Fatalf("expected incoming key returned when all unhealthy, got %q", got)
	}
}

func TestSingleKeyPool(t *testing.T) {
	pool := NewKeyPool([]string{"only-key"})

	// Healthy: should return itself
	got := pool.GetKeyForRequest("only-key")
	if got != "only-key" {
		t.Fatalf("expected only-key, got %q", got)
	}

	// Mark unhealthy: should still return itself (no alternatives)
	pool.RecordError("only-key", 429)
	got = pool.GetKeyForRequest("only-key")
	if got != "only-key" {
		t.Fatalf("expected only-key even when unhealthy (no alternatives), got %q", got)
	}
}

func TestRollingWindowUsagePruning(t *testing.T) {
	savedWindow := keyPoolRollingWindow
	t.Cleanup(func() { keyPoolRollingWindow = savedWindow })
	keyPoolRollingWindow = 100 * time.Millisecond

	pool := NewKeyPool([]string{"key-a"})

	pool.RecordUsage("key-a")
	pool.RecordUsage("key-a")
	pool.RecordUsage("key-a")

	if usage := pool.keys[0].rollingUsage(time.Now()); usage != 3 {
		t.Fatalf("expected 3 usage records, got %d", usage)
	}

	time.Sleep(150 * time.Millisecond)

	// Record one more to trigger pruning
	pool.RecordUsage("key-a")

	pool.mu.Lock()
	remaining := len(pool.keys[0].usageTimes)
	pool.mu.Unlock()

	if remaining != 1 {
		t.Fatalf("expected 1 usage record after pruning, got %d", remaining)
	}
}

func TestStickyKeyPreference(t *testing.T) {
	pool := NewKeyPool([]string{"key-a", "key-b"})

	// key-a is healthy — sticky preference should keep returning it
	for i := 0; i < 10; i++ {
		got := pool.GetKeyForRequest("key-a")
		if got != "key-a" {
			t.Fatalf("iteration %d: expected sticky key-a, got %q", i, got)
		}
	}

	// key-b should also stick when requested
	got := pool.GetKeyForRequest("key-b")
	if got != "key-b" {
		t.Fatalf("expected sticky key-b, got %q", got)
	}
}

func TestNonPoolKeyPassthrough(t *testing.T) {
	pool := NewKeyPool([]string{"key-a", "key-b"})

	// A key not in the pool should pass through unchanged
	got := pool.GetKeyForRequest("external-key")
	if got != "external-key" {
		t.Fatalf("expected external-key passthrough, got %q", got)
	}
}

func TestNewKeyPoolSkipsEmptyKeys(t *testing.T) {
	pool := NewKeyPool([]string{"key-a", "", "key-b", ""})

	if len(pool.keys) != 2 {
		t.Fatalf("expected 2 keys after filtering, got %d", len(pool.keys))
	}

	if pool.keys[0].key != "key-a" || pool.keys[1].key != "key-b" {
		t.Fatalf("unexpected keys: %q, %q", pool.keys[0].key, pool.keys[1].key)
	}
}
