package turbo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wasylq/MSD/site"
)

func TestMatch(t *testing.T) {
	tr := &Turbo{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://turbo.cr/a/_mLcPZ7SPCu", true},
		{"https://turbo.cr/d/l5fbTubNXfPzZ", true},
		{"https://turbo.cr/v/8GQZJdtoY7jKt", true},
		{"https://www.turbo.cr/a/abc_123-def", true},
		{"https://turbo.cr/a/", false},
		{"https://turbo.cr/x/abc", false},
		{"https://other.test/a/abc", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := tr.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolveAlbum(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/a/testalbum" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`
			<html>
				<head><title>Testing - turbo.cr</title></head>
				<body>
					<h1>Testing</h1>
					<table>
						<tr class="file-row" data-id="f1" data-name="cat fails.mp4" data-size="62286348"></tr>
						<tr class="file-row" data-id="f2" data-name="Cat fails2.mp4" data-size="193842875"></tr>
					</table>
				</body>
			</html>`))
	}))
	defer ts.Close()

	tr := &Turbo{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := tr.Resolve(context.Background(), "https://turbo.cr/a/testalbum", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.ID != "testalbum" {
		t.Errorf("album ID = %q, want %q", album.ID, "testalbum")
	}
	if album.Name != "Testing" {
		t.Errorf("album Name = %q, want %q", album.Name, "Testing")
	}
	if len(album.Files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(album.Files))
	}
	if album.Files[0].ID != "f1" {
		t.Errorf("file[0].ID = %q, want %q", album.Files[0].ID, "f1")
	}
	if album.Files[0].Name != "cat fails.mp4" {
		t.Errorf("file[0].Name = %q, want %q", album.Files[0].Name, "cat fails.mp4")
	}
	if album.Files[1].Size != 193842875 {
		t.Errorf("file[1].Size = %d, want %d", album.Files[1].Size, 193842875)
	}
}

func TestResolveAlbum_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body><h1>Empty</h1></body></html>"))
	}))
	defer ts.Close()

	tr := &Turbo{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := tr.Resolve(context.Background(), "https://turbo.cr/a/empty", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolveAlbum_RateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	tr := &Turbo{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := tr.Resolve(context.Background(), "https://turbo.cr/a/limited", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestResolveFile(t *testing.T) {
	ts := turboSignServer(t, http.StatusOK, signResponse{
		Success:          true,
		URL:              "https://cdn.test/data/f1.mp4?token=abc",
		Filename:         "f1.mp4",
		OriginalFilename: "cat fails.mp4",
	})
	defer ts.Close()

	tr := &Turbo{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := tr.Resolve(context.Background(), "https://turbo.cr/d/f1", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(album.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(album.Files))
	}
	if album.Files[0].Name != "cat fails.mp4" {
		t.Errorf("file name = %q, want %q", album.Files[0].Name, "cat fails.mp4")
	}
	if album.Files[0].Size != -1 {
		t.Errorf("file size = %d, want -1", album.Files[0].Size)
	}
}

func TestDownloadRequest(t *testing.T) {
	ts := turboSignServer(t, http.StatusOK, signResponse{
		Success: true,
		URL:     "https://cdn.test/data/f1.mp4?exp=1&token=abc",
	})
	defer ts.Close()

	tr := &Turbo{HTTPClient: ts.Client(), BaseURL: ts.URL}
	req, err := tr.DownloadRequest(context.Background(), site.File{ID: "f1", Name: "cat fails.mp4"})
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}

	got, err := url.Parse(req.URL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	if got.Scheme != "https" || got.Host != "cdn.test" || got.Path != "/data/f1.mp4" {
		t.Errorf("download URL = %q", req.URL)
	}
	if got.Query().Get("dl") != "1" {
		t.Errorf("dl query = %q, want 1", got.Query().Get("dl"))
	}
	if got.Query().Get("token") != "abc" {
		t.Errorf("token query = %q, want abc", got.Query().Get("token"))
	}
	if req.Headers.Get("User-Agent") == "" {
		t.Error("expected User-Agent header")
	}
	if req.Headers.Get("Referer") != ts.URL+"/d/f1" {
		t.Errorf("Referer = %q, want %q", req.Headers.Get("Referer"), ts.URL+"/d/f1")
	}
}

func TestDownloadRequest_Challenge(t *testing.T) {
	ts := turboSignServer(t, http.StatusOK, signResponse{
		Success: false,
		Error:   "captcha required",
	})
	defer ts.Close()

	tr := &Turbo{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := tr.DownloadRequest(context.Background(), site.File{ID: "f1", Name: "cat fails.mp4"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsAuthRequired(err) {
		t.Errorf("expected ErrAuthRequired, got: %v", err)
	}
}

func TestName(t *testing.T) {
	tr := &Turbo{}
	if tr.Name() != "turbo" {
		t.Errorf("Name() = %q, want %q", tr.Name(), "turbo")
	}
}

func TestDefaults(t *testing.T) {
	tr := &Turbo{}
	if tr.DefaultConcurrency() != 2 {
		t.Errorf("DefaultConcurrency() = %d, want 2", tr.DefaultConcurrency())
	}
	if tr.DefaultResolveDelay() != 5*1e9 {
		t.Errorf("DefaultResolveDelay() = %v, want 5s", tr.DefaultResolveDelay())
	}
	if tr.DefaultDownloadDelay() != 2*1e9 {
		t.Errorf("DefaultDownloadDelay() = %v, want 2s", tr.DefaultDownloadDelay())
	}
}

func turboSignServer(t *testing.T, status int, response signResponse) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sign" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("v") != "f1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		}
	}))
}
