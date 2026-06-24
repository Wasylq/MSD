package engine

import (
	"context"
	"errors"
	"log"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"time"

	"github.com/Wasylq/MSD/site"
)

type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  2 * time.Second,
	}
}

func (rp RetryPolicy) Do(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := range rp.MaxRetries + 1 {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !IsRetryable(lastErr) {
			return lastErr
		}
		if attempt < rp.MaxRetries {
			delay := rp.backoff(attempt, lastErr)
			log.Printf("retryable error on attempt %d/%d: %v; retrying in %s", attempt+1, rp.MaxRetries+1, lastErr, delay.Round(time.Millisecond))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return lastErr
}

func (rp RetryPolicy) backoff(attempt int, err error) time.Duration {
	base := float64(rp.BaseDelay)
	if errors.Is(err, site.ErrRateLimited) {
		base *= 5
	}
	delay := base * math.Pow(2, float64(attempt))
	jitter := delay * 0.5 * rand.Float64()
	return time.Duration(delay + jitter)
}

type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return "HTTP " + http.StatusText(e.StatusCode)
}

var errRangeReset = errors.New("range not satisfiable, restarting download")

func IsRetryable(err error) bool {
	if errors.Is(err, site.ErrRateLimited) || errors.Is(err, errRangeReset) {
		return true
	}
	if errors.Is(err, site.ErrNotFound) ||
		errors.Is(err, site.ErrAuthRequired) ||
		errors.Is(err, site.ErrSiteChanged) {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusTooManyRequests || httpErr.StatusCode >= 500
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
