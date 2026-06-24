package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/Wasylq/MSD/engine"
	"github.com/Wasylq/MSD/internal/config"
	"github.com/Wasylq/MSD/site"
	sitekemono "github.com/Wasylq/MSD/site/kemono"

	_ "github.com/Wasylq/MSD/site/filester"
	_ "github.com/Wasylq/MSD/site/gofile"
	_ "github.com/Wasylq/MSD/site/pixeldrain"
	_ "github.com/Wasylq/MSD/site/turbo"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "msd <url>... [flags]",
		Short: "Multi Site Downloader — download albums from file-hosting sites",
		Args:  cobra.MinimumNArgs(1),
		RunE:  run,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.Version = version + " (" + commit + ", " + date + ")"

	f := rootCmd.Flags()
	f.StringP("output", "o", "", "download directory")
	f.IntP("concurrency", "c", 0, "max concurrent downloads")
	f.String("request-delay", "", "delay between requests (e.g. 5s)")
	f.String("password", "", "album password")
	f.Bool("no-resume", false, "don't resume partial downloads")
	f.Bool("dry-run", false, "list files without downloading")
	f.Bool("kemono-thumbnails", false, "download Kemono thumbnails instead of original attachment files")
	f.CountP("debug", "d", "debug output to stderr (repeat for more)")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	verbosity, _ := cmd.Flags().GetCount("debug")
	if verbosity > 0 {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.Ltime)
	} else {
		log.SetOutput(nopWriter{})
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if v, _ := cmd.Flags().GetString("output"); v != "" {
		cfg.DownloadDir = v
	}
	if v, _ := cmd.Flags().GetInt("concurrency"); v > 0 {
		cfg.Concurrency = v
	}
	if v, _ := cmd.Flags().GetString("request-delay"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid --request-delay: %w", err)
		}
		cfg.RequestDelay = d
	}
	if v, _ := cmd.Flags().GetBool("no-resume"); v {
		cfg.NoResume = true
	}

	password, _ := cmd.Flags().GetString("password")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var errs []error
	for _, url := range args {
		if ctx.Err() != nil {
			break
		}
		kemonoThumbnails, _ := cmd.Flags().GetBool("kemono-thumbnails")
		if err := processURL(ctx, cfg, url, password, dryRun, kemonoThumbnails); err != nil {
			fmt.Fprintf(os.Stderr, "Error [%s]: %v\n", url, err)
			errs = append(errs, err)
		}
	}

	if len(errs) == 1 {
		return errs[0]
	}
	if len(errs) > 1 {
		return fmt.Errorf("%d of %d URLs failed", len(errs), len(args))
	}
	return nil
}

func processURL(ctx context.Context, cfg *config.Config, url, password string, dryRun bool, kemonoThumbnails bool) error {
	s := site.Match(url)
	if s == nil {
		return fmt.Errorf("no site handler matches URL: %s", url)
	}
	if k, ok := s.(*sitekemono.Kemono); ok && kemonoThumbnails {
		k.UseThumbnails = true
		log.Printf("kemono thumbnail mode enabled")
	}
	log.Printf("matched site: %s", s.Name())

	log.Printf("resolving: %s", url)
	album, err := s.Resolve(ctx, url, password)
	if err != nil {
		return mapSiteError(err)
	}

	label := album.Name
	if label == "" {
		label = album.Files[0].Name
	}
	fmt.Fprintf(os.Stderr, "%s (%d files)\n", label, len(album.Files))

	if dryRun {
		return printFileTable(album)
	}

	progress := newProgressReporter(album)
	defer progress.Close()

	e := &engine.Engine{
		OutputDir:     cfg.DownloadDir,
		Concurrency:   cfg.Concurrency,
		DownloadDelay: cfg.RequestDelay,
		Retry:         cliRetryPolicy(),
		Progress:      progress,
		HTTPClient:    cliHTTPClient(),
		NoResume:      cfg.NoResume,
	}

	return e.Download(ctx, s, album)
}

func cliRetryPolicy() engine.RetryPolicy {
	return engine.RetryPolicy{
		MaxRetries: 2,
		BaseDelay:  time.Second,
	}
}

func cliHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   8 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   8 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

func printFileTable(album *site.Album) error {
	var totalSize int64
	for _, f := range album.Files {
		size := "unknown"
		if f.Size > 0 {
			size = formatSize(f.Size)
			totalSize += f.Size
		}
		fmt.Fprintf(os.Stderr, "  %-60s %s\n", f.Name, size)
	}
	if totalSize > 0 {
		fmt.Fprintf(os.Stderr, "\nTotal: %s\n", formatSize(totalSize))
	}
	return nil
}

func formatSize(b int64) string {
	const (
		kB = 1024
		mB = 1024 * kB
		gB = 1024 * mB
	)
	switch {
	case b >= gB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gB))
	case b >= mB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mB))
	case b >= kB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func mapSiteError(err error) error {
	switch {
	case site.IsNotFound(err):
		return fmt.Errorf("album or file not found — check the URL")
	case site.IsAuthRequired(err):
		return fmt.Errorf("authentication required — try --password for protected albums or configure site credentials: %w", err)
	case site.IsRateLimited(err):
		return fmt.Errorf("rate limited by site — try again later")
	default:
		return err
	}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
