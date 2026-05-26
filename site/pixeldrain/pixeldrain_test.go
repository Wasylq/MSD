package pixeldrain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wasylq/MSD/site"
)

func TestMatch(t *testing.T) {
	p := &Pixeldrain{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://pixeldrain.com/l/abc123", true},
		{"http://pixeldrain.com/l/XyZ789", true},
		{"https://pixeldrain.com/l/a1B2c3", true},
		{"https://pixeldrain.com/u/abc123", true},
		{"http://pixeldrain.com/u/XyZ789", true},
		{"https://other.com/l/abc123", false},
		{"https://pixeldrain.com/l/", false},
		{"https://pixeldrain.com/u/", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := p.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func newTestPixeldrain(ts *httptest.Server) *Pixeldrain {
	return &Pixeldrain{
		HTTPClient: ts.Client(),
		BaseURL:    ts.URL,
	}
}

func TestResolve(t *testing.T) {
	resp := listResponse{
		ID:    "testlist",
		Title: "Test Album",
		Files: []fileResponse{
			{ID: "f1", Name: "photo.jpg", Size: 1024},
			{ID: "f2", Name: "video.mp4", Size: 2048},
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/list/testlist" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newTestPixeldrain(ts)

	album, err := p.Resolve(context.Background(), "https://pixeldrain.com/l/testlist", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.ID != "testlist" {
		t.Errorf("album ID = %q, want %q", album.ID, "testlist")
	}
	if album.Name != "Test Album" {
		t.Errorf("album Name = %q, want %q", album.Name, "Test Album")
	}
	if len(album.Files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(album.Files))
	}
	if album.Files[0].Name != "photo.jpg" {
		t.Errorf("file[0].Name = %q, want %q", album.Files[0].Name, "photo.jpg")
	}
	if album.Files[0].ID != "f1" {
		t.Errorf("file[0].ID = %q, want %q", album.Files[0].ID, "f1")
	}
	if album.Files[1].Size != 2048 {
		t.Errorf("file[1].Size = %d, want %d", album.Files[1].Size, 2048)
	}
}

func TestResolve_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	p := newTestPixeldrain(ts)
	_, err := p.Resolve(context.Background(), "https://pixeldrain.com/l/missing", "")
	if err == nil {
		t.Fatal("expected error for missing list")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolve_RateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	p := newTestPixeldrain(ts)
	_, err := p.Resolve(context.Background(), "https://pixeldrain.com/l/ratelimited", "")
	if err == nil {
		t.Fatal("expected error for rate-limited response")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestResolve_BadURL(t *testing.T) {
	p := &Pixeldrain{}
	_, err := p.Resolve(context.Background(), "https://other.com/nope", "")
	if err == nil {
		t.Fatal("expected error for non-matching URL")
	}
}

func TestResolve_EmptyAlbum(t *testing.T) {
	resp := listResponse{
		ID:    "empty",
		Title: "Empty Album",
		Files: nil,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newTestPixeldrain(ts)
	album, err := p.Resolve(context.Background(), "https://pixeldrain.com/l/empty", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(album.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(album.Files))
	}
}

func TestResolveFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/file/singlefile/info" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(fileResponse{
			ID:   "singlefile",
			Name: "document.pdf",
			Size: 4096,
		})
	}))
	defer ts.Close()

	p := newTestPixeldrain(ts)
	album, err := p.Resolve(context.Background(), "https://pixeldrain.com/u/singlefile", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.Name != "" {
		t.Errorf("single file album Name = %q, want empty", album.Name)
	}
	if len(album.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(album.Files))
	}
	if album.Files[0].ID != "singlefile" {
		t.Errorf("file ID = %q, want %q", album.Files[0].ID, "singlefile")
	}
	if album.Files[0].Name != "document.pdf" {
		t.Errorf("file Name = %q, want %q", album.Files[0].Name, "document.pdf")
	}
	if album.Files[0].Size != 4096 {
		t.Errorf("file Size = %d, want 4096", album.Files[0].Size)
	}
}

func TestResolveFile_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	p := newTestPixeldrain(ts)
	_, err := p.Resolve(context.Background(), "https://pixeldrain.com/u/missing", "")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDownloadRequest(t *testing.T) {
	p := &Pixeldrain{}
	file := site.File{ID: "abc123", Name: "test.txt", Size: 100}

	req, err := p.DownloadRequest(context.Background(), file)
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}

	expected := defaultBaseURL + "/api/file/abc123?download"
	if req.URL != expected {
		t.Errorf("URL = %q, want %q", req.URL, expected)
	}
	if req.Headers != nil {
		t.Error("expected no custom headers for pixeldrain")
	}
	if req.Cookies != nil {
		t.Error("expected no cookies for pixeldrain")
	}
}

func TestDownloadRequest_CustomBase(t *testing.T) {
	p := &Pixeldrain{BaseURL: "http://localhost:9999"}
	file := site.File{ID: "xyz", Name: "test.txt", Size: 100}

	req, err := p.DownloadRequest(context.Background(), file)
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if req.URL != "http://localhost:9999/api/file/xyz?download" {
		t.Errorf("URL = %q", req.URL)
	}
}

func TestName(t *testing.T) {
	p := &Pixeldrain{}
	if p.Name() != "pixeldrain" {
		t.Errorf("Name() = %q, want %q", p.Name(), "pixeldrain")
	}
}

func TestDefaults(t *testing.T) {
	p := &Pixeldrain{}
	if p.DefaultConcurrency() != 5 {
		t.Errorf("DefaultConcurrency() = %d, want 5", p.DefaultConcurrency())
	}
	if p.DefaultResolveDelay() != 0 {
		t.Errorf("DefaultResolveDelay() = %v, want 0", p.DefaultResolveDelay())
	}
	if p.DefaultDownloadDelay() != 0 {
		t.Errorf("DefaultDownloadDelay() = %v, want 0", p.DefaultDownloadDelay())
	}
}
