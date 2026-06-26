package cyberdrop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wasylq/MSD/site"
)

func TestMatch(t *testing.T) {
	c := &Cyberdrop{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://cyberdrop.cr/f/B00W6EWypNyi7", true},
		{"https://www.cyberdrop.cr/f/B00W6EWypNyi7", true},
		{"http://cyberdrop.cr/f/abc_123-XYZ", true},
		{"https://cyberdrop.cr/f/", false},
		{"https://cyberdrop.cr/f/B00W6EWypNyi7/extra", false},
		{"https://cyberdrop.cr/a/B00W6EWypNyi7", false},
		{"https://other.com/f/B00W6EWypNyi7", false},
		{"not a url", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := c.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/file/info/B00W6EWypNyi7" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected User-Agent header")
		}
		if err := json.NewEncoder(w).Encode(fileInfo{
			Name: "archive.zip",
			Type: "application/zip",
			Size: 6086744,
			Slug: "B00W6EWypNyi7",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	c := &Cyberdrop{HTTPClient: ts.Client(), BaseURL: "https://cyberdrop.cr", APIURL: ts.URL}
	album, err := c.Resolve(context.Background(), "https://cyberdrop.cr/f/B00W6EWypNyi7", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.ID != "B00W6EWypNyi7" {
		t.Errorf("album ID = %q", album.ID)
	}
	if album.Name != "" {
		t.Errorf("single-file album Name = %q, want empty", album.Name)
	}
	if len(album.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(album.Files))
	}
	if album.Files[0].ID != "B00W6EWypNyi7" {
		t.Errorf("file ID = %q", album.Files[0].ID)
	}
	if album.Files[0].Name != "archive.zip" {
		t.Errorf("file Name = %q", album.Files[0].Name)
	}
	if album.Files[0].Size != 6086744 {
		t.Errorf("file Size = %d", album.Files[0].Size)
	}
}

func TestResolve_NotFoundErrorBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(fileInfo{Error: "File not found"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	c := &Cyberdrop{HTTPClient: ts.Client(), APIURL: ts.URL}
	_, err := c.Resolve(context.Background(), "https://cyberdrop.cr/f/missing", "")
	if err == nil {
		t.Fatal("expected error")
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

	c := &Cyberdrop{HTTPClient: ts.Client(), APIURL: ts.URL}
	_, err := c.Resolve(context.Background(), "https://cyberdrop.cr/f/limited", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestResolve_BadURL(t *testing.T) {
	c := &Cyberdrop{}
	_, err := c.Resolve(context.Background(), "https://other.com/nope", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDownloadRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/file/auth/B00W6EWypNyi7" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(authResponse{
			URL: "https://cdn.test/api/file/d/B00W6EWypNyi7?token=signed",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	c := &Cyberdrop{HTTPClient: ts.Client(), BaseURL: "https://cyberdrop.cr", APIURL: ts.URL}
	req, err := c.DownloadRequest(context.Background(), site.File{ID: "B00W6EWypNyi7", Name: "archive.zip"})
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}

	if req.URL != "https://cdn.test/api/file/d/B00W6EWypNyi7?token=signed" {
		t.Errorf("URL = %q", req.URL)
	}
	if req.Headers.Get("User-Agent") == "" {
		t.Error("expected User-Agent header")
	}
	if req.Headers.Get("Referer") != "https://cyberdrop.cr/f/B00W6EWypNyi7" {
		t.Errorf("Referer = %q", req.Headers.Get("Referer"))
	}
}

func TestDownloadRequest_NotFoundErrorBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(authResponse{Error: "Failed to generate signed URL"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	c := &Cyberdrop{HTTPClient: ts.Client(), APIURL: ts.URL}
	_, err := c.DownloadRequest(context.Background(), site.File{ID: "missing", Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDownloadRequest_SiteChanged(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(authResponse{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	c := &Cyberdrop{HTTPClient: ts.Client(), APIURL: ts.URL}
	_, err := c.DownloadRequest(context.Background(), site.File{ID: "missing", Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestName(t *testing.T) {
	c := &Cyberdrop{}
	if c.Name() != "cyberdrop" {
		t.Errorf("Name() = %q", c.Name())
	}
}

func TestDefaults(t *testing.T) {
	c := &Cyberdrop{}
	if c.DefaultConcurrency() != 3 {
		t.Errorf("DefaultConcurrency() = %d, want 3", c.DefaultConcurrency())
	}
	if c.DefaultResolveDelay() != 0 {
		t.Errorf("DefaultResolveDelay() = %v, want 0", c.DefaultResolveDelay())
	}
	if c.DefaultDownloadDelay() != time.Second {
		t.Errorf("DefaultDownloadDelay() = %v, want 1s", c.DefaultDownloadDelay())
	}
}
