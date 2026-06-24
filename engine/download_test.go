package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wasylq/MSD/site"
)

func newTestServer(content map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		data, ok := content[id]
		if !ok {
			http.NotFound(w, r)
			return
		}

		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			var start int64
			_, _ = fmt.Sscanf(rangeHeader, "bytes=%d-", &start)
			if start >= int64(len(data)) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", int64(len(data))-start))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(data[start:]))
			return
		}

		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		_, _ = w.Write([]byte(data))
	}))
}

type mockSite struct {
	serverURL     string
	concurrency   int
	downloadDelay time.Duration
	dlReqErr      error
}

func (m *mockSite) Name() string                                                 { return "mock" }
func (m *mockSite) Match(string) bool                                            { return true }
func (m *mockSite) Resolve(context.Context, string, string) (*site.Album, error) { return nil, nil }
func (m *mockSite) DefaultConcurrency() int                                      { return m.concurrency }
func (m *mockSite) DefaultResolveDelay() time.Duration                           { return 0 }
func (m *mockSite) DefaultDownloadDelay() time.Duration                          { return m.downloadDelay }

func (m *mockSite) DownloadRequest(_ context.Context, file site.File) (*site.DownloadRequest, error) {
	if m.dlReqErr != nil {
		return nil, m.dlReqErr
	}
	return &site.DownloadRequest{
		URL: m.serverURL + "/" + file.ID,
	}, nil
}

func TestDownloadFile_Basic(t *testing.T) {
	content := map[string]string{"file1": "hello world"}
	ts := newTestServer(content)
	defer ts.Close()

	dir := t.TempDir()
	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress: NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	file := site.File{ID: "file1", Name: "test.txt", Size: 11}

	if err := e.downloadFile(context.Background(), s, file, dir); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", got, "hello world")
	}

	// .part file should not exist
	if _, err := os.Stat(filepath.Join(dir, "test.txt.part")); !os.IsNotExist(err) {
		t.Error(".part file should not exist after successful download")
	}
}

func TestDownloadFile_SkipsCompleted(t *testing.T) {
	content := map[string]string{"file1": "hello world"}
	ts := newTestServer(content)
	defer ts.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(destPath, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	requestCount := 0
	origHandler := ts.Config.Handler
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		origHandler.ServeHTTP(w, r)
	})

	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress: NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	file := site.File{ID: "file1", Name: "test.txt", Size: 11}

	if err := e.downloadFile(context.Background(), s, file, dir); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	if requestCount != 0 {
		t.Errorf("expected 0 HTTP requests for completed file, got %d", requestCount)
	}
}

func TestDownloadFile_SkipsUnknownSizeIfExists(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(destPath, []byte("existing content"), 0o644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress: NoopReporter{},
	}
	s := &mockSite{serverURL: "http://unused", concurrency: 1}
	file := site.File{ID: "file1", Name: "test.txt", Size: -1}

	if err := e.downloadFile(context.Background(), s, file, dir); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(got) != "existing content" {
		t.Errorf("file content changed; expected skip")
	}
}

func TestDownloadFile_Resume(t *testing.T) {
	content := map[string]string{"file1": "hello world"}
	ts := newTestServer(content)
	defer ts.Close()

	dir := t.TempDir()
	partPath := filepath.Join(dir, "test.txt.part")
	if err := os.WriteFile(partPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write part file: %v", err)
	}

	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress: NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	file := site.File{ID: "file1", Name: "test.txt", Size: 11}

	if err := e.downloadFile(context.Background(), s, file, dir); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", got, "hello world")
	}
}

func TestDownloadFile_NoResumeFlag(t *testing.T) {
	content := map[string]string{"file1": "hello world"}
	ts := newTestServer(content)
	defer ts.Close()

	dir := t.TempDir()
	partPath := filepath.Join(dir, "test.txt.part")
	if err := os.WriteFile(partPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write part file: %v", err)
	}

	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress: NoopReporter{},
		NoResume: true,
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	file := site.File{ID: "file1", Name: "test.txt", Size: 11}

	if err := e.downloadFile(context.Background(), s, file, dir); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", got, "hello world")
	}
}

func TestDownloadFile_RangeNotSatisfiable(t *testing.T) {
	content := map[string]string{"file1": "hello world"}
	ts := newTestServer(content)
	defer ts.Close()

	dir := t.TempDir()
	// .part file larger than actual content — triggers 416
	partPath := filepath.Join(dir, "test.txt.part")
	if err := os.WriteFile(partPath, []byte("hello world plus extra garbage"), 0o644); err != nil {
		t.Fatalf("write part file: %v", err)
	}

	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 1, BaseDelay: time.Millisecond},
		Progress: NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	file := site.File{ID: "file1", Name: "test.txt", Size: 11}

	if err := e.downloadFile(context.Background(), s, file, dir); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", got, "hello world")
	}
}

func TestDownloadFile_NotFound(t *testing.T) {
	ts := newTestServer(map[string]string{})
	defer ts.Close()

	dir := t.TempDir()
	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress: NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	file := site.File{ID: "missing", Name: "test.txt", Size: 100}

	err := e.downloadFile(context.Background(), s, file, dir)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDownloadFile_SizeMismatch(t *testing.T) {
	content := map[string]string{"file1": "short"}
	ts := newTestServer(content)
	defer ts.Close()

	dir := t.TempDir()
	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress: NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	file := site.File{ID: "file1", Name: "test.txt", Size: 999}

	err := e.downloadFile(context.Background(), s, file, dir)
	if err == nil {
		t.Fatal("expected size mismatch error")
	}
	if !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("expected size mismatch error, got: %v", err)
	}
}

func TestDownloadFile_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		// Write slowly to allow cancellation
		for i := 0; i < 1000; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
				_, _ = w.Write([]byte(strings.Repeat("x", 1000)))
				w.(http.Flusher).Flush()
				time.Sleep(time.Millisecond)
			}
		}
	}))
	defer ts.Close()

	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress: NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	file := site.File{ID: "file1", Name: "test.txt", Size: 1000000}

	err := e.downloadFile(ctx, s, file, dir)
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

func TestDownloadFile_ProgressCallbacks(t *testing.T) {
	content := map[string]string{"file1": "hello"}
	ts := newTestServer(content)
	defer ts.Close()

	dir := t.TempDir()
	pr := &recordingReporter{}
	e := &Engine{
		Retry:    RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress: pr,
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	file := site.File{ID: "file1", Name: "test.txt", Size: 5}

	if err := e.downloadFile(context.Background(), s, file, dir); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}

	if v := pr.starts.Load(); v != 1 {
		t.Errorf("OnFileStart called %d times, want 1", v)
	}
	if v := pr.completes.Load(); v != 1 {
		t.Errorf("OnFileComplete called %d times, want 1", v)
	}
	if pr.progressCalls.Load() == 0 {
		t.Error("OnFileProgress never called")
	}
	pr.mu.Lock()
	lastErr := pr.lastErr
	pr.mu.Unlock()
	if lastErr != nil {
		t.Errorf("OnFileComplete got error: %v", lastErr)
	}
}

type recordingReporter struct {
	starts        atomic.Int32
	completes     atomic.Int32
	progressCalls atomic.Int32
	albumDone     atomic.Bool
	succeeded     atomic.Int32
	failed        atomic.Int32

	mu      sync.Mutex
	lastErr error
}

func (r *recordingReporter) OnFileStart(site.File)                  { r.starts.Add(1) }
func (r *recordingReporter) OnFileProgress(site.File, int64, int64) { r.progressCalls.Add(1) }
func (r *recordingReporter) OnFileComplete(_ site.File, err error) {
	r.completes.Add(1)
	r.mu.Lock()
	r.lastErr = err
	r.mu.Unlock()
}
func (r *recordingReporter) OnAlbumComplete(_ site.Album, s, f int) {
	r.albumDone.Store(true)
	r.succeeded.Store(int32(s))
	r.failed.Store(int32(f))
}
