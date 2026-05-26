package engine

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wasylq/MSD/site"
)

func TestRetryPolicy_SucceedsImmediately(t *testing.T) {
	rp := RetryPolicy{MaxRetries: 3, BaseDelay: time.Millisecond}
	calls := 0
	err := rp.Do(context.Background(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryPolicy_RetriesOnRetryableError(t *testing.T) {
	rp := RetryPolicy{MaxRetries: 3, BaseDelay: time.Millisecond}
	calls := 0
	err := rp.Do(context.Background(), func() error {
		calls++
		if calls < 3 {
			return &HTTPError{StatusCode: http.StatusInternalServerError}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryPolicy_StopsOnNonRetryable(t *testing.T) {
	rp := RetryPolicy{MaxRetries: 3, BaseDelay: time.Millisecond}
	calls := 0
	err := rp.Do(context.Background(), func() error {
		calls++
		return site.ErrNotFound
	})
	if !errors.Is(err, site.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryPolicy_ExhaustsRetries(t *testing.T) {
	rp := RetryPolicy{MaxRetries: 2, BaseDelay: time.Millisecond}
	calls := 0
	serverErr := &HTTPError{StatusCode: http.StatusBadGateway}
	err := rp.Do(context.Background(), func() error {
		calls++
		return serverErr
	})
	if !errors.As(err, new(*HTTPError)) {
		t.Fatalf("expected HTTPError, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (1 + 2 retries), got %d", calls)
	}
}

func TestRetryPolicy_RespectsContextCancellation(t *testing.T) {
	rp := RetryPolicy{MaxRetries: 10, BaseDelay: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	err := rp.Do(ctx, func() error {
		calls++
		return site.ErrRateLimited
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"rate limited", site.ErrRateLimited, true},
		{"range reset", errRangeReset, true},
		{"server error 500", &HTTPError{StatusCode: 500}, true},
		{"server error 502", &HTTPError{StatusCode: 502}, true},
		{"too many requests 429", &HTTPError{StatusCode: 429}, true},
		{"not found site", site.ErrNotFound, false},
		{"auth required", site.ErrAuthRequired, false},
		{"site changed", site.ErrSiteChanged, false},
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		{"client error 400", &HTTPError{StatusCode: 400}, false},
		{"client error 403", &HTTPError{StatusCode: 403}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.retryable {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}
