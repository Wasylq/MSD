package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/Wasylq/MSD/internal/fsutil"
	"github.com/Wasylq/MSD/site"
)

type Engine struct {
	OutputDir     string
	Concurrency   int
	ResolveDelay  time.Duration
	DownloadDelay time.Duration
	Retry         RetryPolicy
	Progress      ProgressReporter
	HTTPClient    *http.Client
	NoResume      bool
}

func (e *Engine) httpClient() *http.Client {
	if e.HTTPClient != nil {
		return e.HTTPClient
	}
	return http.DefaultClient
}

func (e *Engine) progress() ProgressReporter {
	if e.Progress != nil {
		return e.Progress
	}
	return NoopReporter{}
}

func (e *Engine) Download(ctx context.Context, s site.Site, album *site.Album) error {
	dir := filepath.Join(e.OutputDir, fsutil.SanitizePath(album.Name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	concurrency := e.Concurrency
	if concurrency <= 0 {
		concurrency = s.DefaultConcurrency()
	}
	if concurrency <= 0 {
		concurrency = 1
	}

	var limiter *rate.Limiter
	delay := e.DownloadDelay
	if delay <= 0 {
		delay = s.DefaultDownloadDelay()
	}
	if delay > 0 {
		limiter = rate.NewLimiter(rate.Every(delay), 1)
	}

	var g errgroup.Group
	g.SetLimit(concurrency)

	var succeeded, failed atomic.Int32
	for i := range album.Files {
		file := album.Files[i]
		file.Name = fsutil.SanitizeName(file.Name)

		g.Go(func() error {
			if ctx.Err() != nil {
				return nil
			}
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					return nil
				}
			}
			err := e.downloadFile(ctx, s, file, dir)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					failed.Add(1)
				}
				return nil
			}
			succeeded.Add(1)
			return nil
		})
	}

	g.Wait()
	e.progress().OnAlbumComplete(*album, int(succeeded.Load()), int(failed.Load()))

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if failed.Load() > 0 {
		return fmt.Errorf("%d of %d files failed to download", failed.Load(), len(album.Files))
	}
	return nil
}
