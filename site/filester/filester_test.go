package filester

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Wasylq/MSD/site"
)

func TestMatch(t *testing.T) {
	f := &Filester{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://filester.me/f/abc123", true},
		{"https://filester.me/f/3de7fbc9228bb07f", true},
		{"https://filester.me/f/slug-with-dashes", true},
		{"https://filester.me/f/slug_with_underscores", true},
		{"https://filester.me/d/abc123", false},
		{"https://other.com/f/abc123", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := f.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func serveFixture(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(data)
	}
}

func TestResolve_SinglePage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/f/testalbum" {
			http.NotFound(w, r)
			return
		}
		// Serve page2 fixture (no pagination link) for any page request
		data, err := os.ReadFile("testdata/album_page2.html")
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(data)
	}))
	defer ts.Close()

	f := &Filester{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := f.Resolve(context.Background(), "https://filester.me/f/testalbum", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.Name != "Test Album" {
		t.Errorf("album Name = %q, want %q", album.Name, "Test Album")
	}
	if len(album.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(album.Files))
	}
	if album.Files[0].ID != "ghi789" {
		t.Errorf("file ID = %q, want %q", album.Files[0].ID, "ghi789")
	}
	if album.Files[0].Name != "document.pdf" {
		t.Errorf("file Name = %q, want %q", album.Files[0].Name, "document.pdf")
	}
	if album.Files[0].Size != 2048 {
		t.Errorf("file Size = %d, want 2048", album.Files[0].Size)
	}
}

func TestResolve_Pagination(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/f/testalbum" {
			http.NotFound(w, r)
			return
		}
		page := r.URL.Query().Get("page")
		var fixture string
		switch page {
		case "1", "":
			fixture = "testdata/album_page1.html"
		default:
			fixture = "testdata/album_page2.html"
		}
		data, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(data)
	}))
	defer ts.Close()

	f := &Filester{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := f.Resolve(context.Background(), "https://filester.me/f/testalbum", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(album.Files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(album.Files))
	}

	expected := []struct {
		id   string
		name string
		size int64
	}{
		{"abc123", "photo1.jpg", 1048576},
		{"def456", "video.mp4", 524288000},
		{"ghi789", "document.pdf", 2048},
	}
	for i, e := range expected {
		if album.Files[i].ID != e.id {
			t.Errorf("file[%d].ID = %q, want %q", i, album.Files[i].ID, e.id)
		}
		if album.Files[i].Name != e.name {
			t.Errorf("file[%d].Name = %q, want %q", i, album.Files[i].Name, e.name)
		}
		if album.Files[i].Size != e.size {
			t.Errorf("file[%d].Size = %d, want %d", i, album.Files[i].Size, e.size)
		}
	}
}

func TestResolve_Empty(t *testing.T) {
	ts := httptest.NewServer(serveFixture("testdata/album_empty.html"))
	defer ts.Close()

	f := &Filester{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := f.Resolve(context.Background(), "https://filester.me/f/emptyalbum", "")
	if err == nil {
		t.Fatal("expected error for empty album")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolve_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	f := &Filester{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := f.Resolve(context.Background(), "https://filester.me/f/missing", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolve_RateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	f := &Filester{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := f.Resolve(context.Background(), "https://filester.me/f/limited", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestResolve_BadURL(t *testing.T) {
	f := &Filester{}
	_, err := f.Resolve(context.Background(), "https://other.com/nope", "")
	if err == nil {
		t.Fatal("expected error for non-matching URL")
	}
}

func TestDownloadRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/view" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["file_slug"] != "abc123" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"view_url": "/v/token123",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	f := &Filester{HTTPClient: ts.Client(), BaseURL: ts.URL, CDNURL: "https://cdn.test"}
	file := site.File{ID: "abc123", Name: "test.txt", Size: 100}

	req, err := f.DownloadRequest(context.Background(), file)
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}

	if req.URL != "https://cdn.test/v/token123" {
		t.Errorf("URL = %q, want %q", req.URL, "https://cdn.test/v/token123")
	}
	if req.Headers.Get("User-Agent") == "" {
		t.Error("expected User-Agent header")
	}
}

func TestDownloadRequest_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": false,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	f := &Filester{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := f.DownloadRequest(context.Background(), site.File{ID: "bad", Name: "x"})
	if err == nil {
		t.Fatal("expected error for failed API response")
	}
}

func TestDownloadRequest_RateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	f := &Filester{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := f.DownloadRequest(context.Background(), site.File{ID: "x", Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestName(t *testing.T) {
	f := &Filester{}
	if f.Name() != "filester" {
		t.Errorf("Name() = %q", f.Name())
	}
}

func TestDefaults(t *testing.T) {
	f := &Filester{}
	if f.DefaultConcurrency() != 3 {
		t.Errorf("DefaultConcurrency() = %d, want 3", f.DefaultConcurrency())
	}
	if f.DefaultResolveDelay() != 5*1e9 {
		t.Errorf("DefaultResolveDelay() = %v, want 5s", f.DefaultResolveDelay())
	}
	if f.DefaultDownloadDelay() != 0 {
		t.Errorf("DefaultDownloadDelay() = %v, want 0", f.DefaultDownloadDelay())
	}
}
