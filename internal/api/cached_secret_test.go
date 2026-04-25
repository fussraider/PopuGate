package api

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockSecretProvider tracks call count for testing.
type mockSecretProvider struct {
	secret string
	err    error
	calls  atomic.Int64
}

func (m *mockSecretProvider) GetJWTSecret(ctx context.Context) (string, error) {
	m.calls.Add(1)
	return m.secret, m.err
}

func TestCachedJWTSecret_ReturnsCached(t *testing.T) {
	mock := &mockSecretProvider{secret: "test-secret"}
	cached := NewCachedJWTSecretProvider(mock, 5*time.Minute)

	// First call hits the source
	s1, err := cached.GetJWTSecret(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s1 != "test-secret" {
		t.Errorf("got %q, want %q", s1, "test-secret")
	}
	if mock.calls.Load() != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls.Load())
	}

	// Second call should use cache
	s2, err := cached.GetJWTSecret(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s2 != "test-secret" {
		t.Errorf("got %q, want %q", s2, "test-secret")
	}
	if mock.calls.Load() != 1 {
		t.Errorf("expected 1 call (cached), got %d", mock.calls.Load())
	}
}

func TestCachedJWTSecret_RefreshesAfterTTL(t *testing.T) {
	mock := &mockSecretProvider{secret: "old-secret"}
	cached := NewCachedJWTSecretProvider(mock, 50*time.Millisecond)

	// Initial fetch
	s, _ := cached.GetJWTSecret(context.Background())
	if s != "old-secret" {
		t.Errorf("got %q, want %q", s, "old-secret")
	}

	// Change the underlying secret
	mock.secret = "new-secret"

	// Still cached
	s, _ = cached.GetJWTSecret(context.Background())
	if s != "old-secret" {
		t.Errorf("got %q, want cached %q", s, "old-secret")
	}

	// Wait for TTL to expire
	time.Sleep(80 * time.Millisecond)

	// Should refresh
	s, _ = cached.GetJWTSecret(context.Background())
	if s != "new-secret" {
		t.Errorf("got %q, want refreshed %q", s, "new-secret")
	}
	if mock.calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", mock.calls.Load())
	}
}

func TestCachedJWTSecret_ReturnsStaleOnSourceError(t *testing.T) {
	mock := &mockSecretProvider{secret: "cached-secret"}
	cached := NewCachedJWTSecretProvider(mock, 50*time.Millisecond)

	// Populate cache
	cached.GetJWTSecret(context.Background())

	// TTL expires, source now returns error
	time.Sleep(80 * time.Millisecond)
	mock.err = context.Canceled

	s, err := cached.GetJWTSecret(context.Background())
	if err != nil {
		t.Fatalf("should return stale value, got error: %v", err)
	}
	if s != "cached-secret" {
		t.Errorf("got %q, want stale %q", s, "cached-secret")
	}
}

func TestCachedJWTSecret_ConcurrentAccess(t *testing.T) {
	mock := &mockSecretProvider{secret: "concurrent-secret"}
	cached := NewCachedJWTSecretProvider(mock, 5*time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := cached.GetJWTSecret(context.Background())
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if s != "concurrent-secret" {
				t.Errorf("got %q, want %q", s, "concurrent-secret")
			}
		}()
	}
	wg.Wait()

	// Only 1 call should have hit the source (double-check locking)
	if mock.calls.Load() != 1 {
		t.Errorf("expected 1 source call, got %d", mock.calls.Load())
	}
}
