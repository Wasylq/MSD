package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wasylq/MSD/site"
)

func newMultiFileServer(files map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		data, ok := files[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write([]byte(data))
	}))
}

func TestEngine_Download_BasicAlbum(t *testing.T) {
	files := map[string]string{
		"a": "file a content",
		"b": "file b content",
		"c": "file c content",
	}
	ts := newMultiFileServer(files)
	defer ts.Close()

	dir := t.TempDir()
	pr := &recordingReporter{}
	e := &Engine{
		OutputDir:   dir,
		Concurrency: 2,
		Retry:       RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress:    pr,
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 3}
	album := &site.Album{
		ID:   "test-album",
		Name: "Test Album",
		Files: []site.File{
			{ID: "a", Name: "a.txt", Size: int64(len(files["a"]))},
			{ID: "b", Name: "b.txt", Size: int64(len(files["b"]))},
			{ID: "c", Name: "c.txt", Size: int64(len(files["c"]))},
		},
	}

	if err := e.Download(context.Background(), s, album); err != nil {
		t.Fatalf("Download: %v", err)
	}

	albumDir := filepath.Join(dir, "Test Album")
	for _, f := range []string{"a.txt", "b.txt", "c.txt"} {
		data, err := os.ReadFile(filepath.Join(albumDir, f))
		if err != nil {
			t.Errorf("read %s: %v", f, err)
			continue
		}
		id := strings.TrimSuffix(f, ".txt")
		if string(data) != files[id] {
			t.Errorf("%s content = %q, want %q", f, data, files[id])
		}
	}

	if !pr.albumDone.Load() {
		t.Error("OnAlbumComplete not called")
	}
	if v := pr.succeeded.Load(); v != 3 {
		t.Errorf("succeeded = %d, want 3", v)
	}
	if v := pr.failed.Load(); v != 0 {
		t.Errorf("failed = %d, want 0", v)
	}
}

func TestEngine_Download_ConcurrencyLimit(t *testing.T) {
	var concurrent, maxConcurrent atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := concurrent.Add(1)
		for {
			prev := maxConcurrent.Load()
			if cur <= prev || maxConcurrent.CompareAndSwap(prev, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		concurrent.Add(-1)
		w.Header().Set("Content-Length", "5")
		w.Write([]byte("hello"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	e := &Engine{
		OutputDir:   dir,
		Concurrency: 2,
		Retry:       RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress:    NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 10}

	var files []site.File
	for i := range 6 {
		files = append(files, site.File{
			ID:   fmt.Sprintf("f%d", i),
			Name: fmt.Sprintf("f%d.txt", i),
			Size: 5,
		})
	}
	album := &site.Album{ID: "album", Name: "album", Files: files}

	if err := e.Download(context.Background(), s, album); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if maxConcurrent.Load() > 2 {
		t.Errorf("max concurrent = %d, want <= 2", maxConcurrent.Load())
	}
}

func TestEngine_Download_UsesSiteDefaultConcurrency(t *testing.T) {
	var concurrent, maxConcurrent atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := concurrent.Add(1)
		for {
			prev := maxConcurrent.Load()
			if cur <= prev || maxConcurrent.CompareAndSwap(prev, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		concurrent.Add(-1)
		w.Header().Set("Content-Length", "5")
		w.Write([]byte("hello"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	e := &Engine{
		OutputDir: dir,
		Retry:     RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress:  NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 3}

	var files []site.File
	for i := range 9 {
		files = append(files, site.File{
			ID:   fmt.Sprintf("f%d", i),
			Name: fmt.Sprintf("f%d.txt", i),
			Size: 5,
		})
	}
	album := &site.Album{ID: "album", Name: "album", Files: files}

	if err := e.Download(context.Background(), s, album); err != nil {
		t.Fatalf("Download: %v", err)
	}

	if maxConcurrent.Load() > 3 {
		t.Errorf("max concurrent = %d, want <= 3", maxConcurrent.Load())
	}
}

func TestEngine_Download_RateLimiting(t *testing.T) {
	files := map[string]string{
		"a": "aaa",
		"b": "bbb",
		"c": "ccc",
	}
	ts := newMultiFileServer(files)
	defer ts.Close()

	dir := t.TempDir()
	e := &Engine{
		OutputDir:     dir,
		Concurrency:   1,
		DownloadDelay: 50 * time.Millisecond,
		Retry:         RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress:      NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	album := &site.Album{
		ID:   "album",
		Name: "album",
		Files: []site.File{
			{ID: "a", Name: "a.txt", Size: 3},
			{ID: "b", Name: "b.txt", Size: 3},
			{ID: "c", Name: "c.txt", Size: 3},
		},
	}

	start := time.Now()
	if err := e.Download(context.Background(), s, album); err != nil {
		t.Fatalf("Download: %v", err)
	}
	elapsed := time.Since(start)

	// 3 files with 50ms delay → at least 100ms total (first doesn't wait)
	if elapsed < 80*time.Millisecond {
		t.Errorf("expected rate limiting delay, elapsed=%v", elapsed)
	}
}

func TestEngine_Download_PartialFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		if id == "bad" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", "4")
		w.Write([]byte("good"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	pr := &recordingReporter{}
	e := &Engine{
		OutputDir:   dir,
		Concurrency: 1,
		Retry:       RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress:    pr,
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	album := &site.Album{
		ID:   "album",
		Name: "album",
		Files: []site.File{
			{ID: "good1", Name: "good1.txt", Size: 4},
			{ID: "bad", Name: "bad.txt", Size: 4},
			{ID: "good2", Name: "good2.txt", Size: 4},
		},
	}

	err := e.Download(context.Background(), s, album)
	if err == nil {
		t.Fatal("expected error for partial failure")
	}
	if !strings.Contains(err.Error(), "1 of 3") {
		t.Errorf("expected '1 of 3 files failed', got: %v", err)
	}

	// Good files should still be downloaded
	if _, err := os.Stat(filepath.Join(dir, "album", "good1.txt")); err != nil {
		t.Error("good1.txt should exist")
	}
	if _, err := os.Stat(filepath.Join(dir, "album", "good2.txt")); err != nil {
		t.Error("good2.txt should exist")
	}

	if v := pr.succeeded.Load(); v != 2 {
		t.Errorf("succeeded = %d, want 2", v)
	}
	if v := pr.failed.Load(); v != 1 {
		t.Errorf("failed = %d, want 1", v)
	}
}

func TestEngine_Download_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.Write([]byte("slow"))
	}))
	defer ts.Close()

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	e := &Engine{
		OutputDir:   dir,
		Concurrency: 1,
		Retry:       RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress:    NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	album := &site.Album{
		ID:   "album",
		Name: "album",
		Files: []site.File{
			{ID: "slow", Name: "slow.txt", Size: 4},
		},
	}

	err := e.Download(ctx, s, album)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestEngine_Download_CreatesAlbumDirectory(t *testing.T) {
	files := map[string]string{"a": "content"}
	ts := newMultiFileServer(files)
	defer ts.Close()

	dir := t.TempDir()
	e := &Engine{
		OutputDir:   dir,
		Concurrency: 1,
		Retry:       RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress:    NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	album := &site.Album{
		ID:   "album",
		Name: "My Great Album",
		Files: []site.File{
			{ID: "a", Name: "a.txt", Size: 7},
		},
	}

	if err := e.Download(context.Background(), s, album); err != nil {
		t.Fatalf("Download: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "My Great Album"))
	if err != nil {
		t.Fatalf("album directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("album path is not a directory")
	}
}

func TestEngine_Download_SanitizesFilenames(t *testing.T) {
	files := map[string]string{"a": "content"}
	ts := newMultiFileServer(files)
	defer ts.Close()

	dir := t.TempDir()
	e := &Engine{
		OutputDir:   dir,
		Concurrency: 1,
		Retry:       RetryPolicy{MaxRetries: 0, BaseDelay: time.Millisecond},
		Progress:    NoopReporter{},
	}
	s := &mockSite{serverURL: ts.URL, concurrency: 1}
	album := &site.Album{
		ID:   "album",
		Name: "album",
		Files: []site.File{
			{ID: "a", Name: "file<with>bad:chars.txt", Size: 7},
		},
	}

	if err := e.Download(context.Background(), s, album); err != nil {
		t.Fatalf("Download: %v", err)
	}

	expected := filepath.Join(dir, "album", "file_with_bad_chars.txt")
	if _, err := os.Stat(expected); err != nil {
		entries, _ := os.ReadDir(filepath.Join(dir, "album"))
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected sanitized file at %s, dir contents: %v", expected, names)
	}
}
