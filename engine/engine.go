package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	dir := e.OutputDir
	if album.Name != "" {
		dir = filepath.Join(dir, fsutil.SanitizePath(album.Name))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := writePostLinks(dir, album.PostLinks); err != nil {
		return fmt.Errorf("write post links: %w", err)
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
					log.Printf("file failed: %s: %v", file.Name, err)
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

func writePostLinks(dir string, links []string) error {
	if len(links) == 0 {
		return nil
	}
	return os.WriteFile(filepath.Join(dir, "post-links.txt"), []byte(strings.Join(links, "\n")+"\n"), 0o644)
}
