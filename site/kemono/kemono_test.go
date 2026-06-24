package kemono

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wasylq/MSD/site"
)

func TestMatch(t *testing.T) {
	k := &Kemono{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://kemono.cr/patreon/user/11111111", true},
		{"https://kemono.cr/fanbox/user/abc_123", true},
		{"https://kemono.cr/patreon/user/11111111?o=50", true},
		{"https://pawchive.st/patreon/user/11111111", true},
		{"https://pawchive.st/patreon/user/11111111?o=50", true},
		{"https://kemono.cr/patreon/post/123", false},
		{"https://other.com/patreon/user/11111111", false},
		{"not a url", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := k.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestPawchiveDownloadURLs(t *testing.T) {
	k := &Kemono{}
	userURL, err := parseUserURL("https://pawchive.st/patreon/user/11111111")
	if err != nil {
		t.Fatalf("parseUserURL: %v", err)
	}

	got := k.dataURL("/24/0e/hash.png", "Sample 1.png", userURL)
	want := "https://file.pawchive.st/data/24/0e/hash.png?f=Sample+1.png"
	if got != want {
		t.Errorf("data URL = %q, want %q", got, want)
	}

	k.UseThumbnails = true
	got = k.dataURL("/24/0e/hash.png", "Sample 1.png", userURL)
	want = "https://img.pawchive.st/thumbnail/data/24/0e/hash.png"
	if got != want {
		t.Errorf("thumbnail URL = %q, want %q", got, want)
	}

	got = postURL(userURL, "22222222")
	want = "https://pawchive.st/patreon/user/11111111/post/22222222"
	if got != want {
		t.Errorf("post URL = %q, want %q", got, want)
	}
}

func TestResolve(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/css" {
			t.Errorf("Accept = %q, want text/css", r.Header.Get("Accept"))
		}

		switch r.URL.Path {
		case "/api/v1/patreon/user/11111111/profile":
			if err := json.NewEncoder(w).Encode(profileResponse{
				ID:      "11111111",
				Name:    "creator",
				Service: "patreon",
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "/api/v1/patreon/user/11111111/posts":
			if r.URL.Query().Get("o") != "0" {
				if err := json.NewEncoder(w).Encode([]postResponse{}); err != nil {
					t.Fatalf("encode response: %v", err)
				}
				return
			}
			if err := json.NewEncoder(w).Encode([]postResponse{
				{
					ID:        "33333333",
					User:      "11111111",
					Service:   "patreon",
					Title:     "Sample",
					Published: "2026-03-18T16:00:12",
					File: kemonoFile{
						Name: "Comic-sample.png",
						Path: "/9e/fa/hash.png",
					},
					Attachments: []kemonoFile{
						{Name: "Comic-sample.png", Path: "/9e/fa/hash.png"},
						{Name: "Extra.png", Path: "/aa/bb/extra.png"},
					},
				},
				{
					ID:          "44444444",
					User:        "11111111",
					Service:     "patreon",
					Title:       "Text only",
					Published:   "2025-12-07T18:26:55",
					Attachments: []kemonoFile{},
				},
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	k := &Kemono{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := k.Resolve(context.Background(), "https://kemono.cr/patreon/user/11111111", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.ID != "patreon-11111111" {
		t.Errorf("album ID = %q, want patreon-11111111", album.ID)
	}
	if album.Name != "kemono-patreon-creator" {
		t.Errorf("album Name = %q, want kemono-patreon-creator", album.Name)
	}
	if len(album.Files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(album.Files))
	}
	wantPostLinks := []string{
		"https://kemono.cr/patreon/user/11111111/post/33333333",
		"https://kemono.cr/patreon/user/11111111/post/44444444",
	}
	if len(album.PostLinks) != len(wantPostLinks) {
		t.Fatalf("len(PostLinks) = %d, want %d", len(album.PostLinks), len(wantPostLinks))
	}
	for i, want := range wantPostLinks {
		if album.PostLinks[i] != want {
			t.Errorf("PostLinks[%d] = %q, want %q", i, album.PostLinks[i], want)
		}
	}

	if album.Files[0].ID != "33333333:/9e/fa/hash.png" {
		t.Errorf("file[0].ID = %q", album.Files[0].ID)
	}
	if !strings.Contains(album.Files[0].Name, "Sample") {
		t.Errorf("file[0].Name = %q, want title included", album.Files[0].Name)
	}
	if album.Files[0].Size != -1 {
		t.Errorf("file[0].Size = %d, want -1", album.Files[0].Size)
	}

	req, err := k.DownloadRequest(context.Background(), album.Files[1])
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if req.URL != ts.URL+"/data/aa/bb/extra.png" {
		t.Errorf("download URL = %q", req.URL)
	}
	if req.Headers.Get("User-Agent") == "" {
		t.Error("expected User-Agent header")
	}
}

func TestResolve_ThumbnailMode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/patreon/user/11111111/profile":
			if err := json.NewEncoder(w).Encode(profileResponse{ID: "11111111", Name: "creator"}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "/api/v1/patreon/user/11111111/posts":
			if err := json.NewEncoder(w).Encode([]postResponse{
				{
					ID:        "33333333",
					Title:     "Sample",
					Published: "2026-03-18T16:00:12",
					Attachments: []kemonoFile{
						{Name: "Comic-sample.png", Path: "/9e/fa/hash.png"},
					},
				},
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	k := &Kemono{
		HTTPClient:       ts.Client(),
		BaseURL:          ts.URL,
		ThumbnailBaseURL: "https://thumb.test",
		UseThumbnails:    true,
	}
	album, err := k.Resolve(context.Background(), "https://kemono.cr/patreon/user/11111111", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req, err := k.DownloadRequest(context.Background(), album.Files[0])
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if req.URL != "https://thumb.test/thumbnail/data/9e/fa/hash.png" {
		t.Errorf("thumbnail URL = %q", req.URL)
	}
}

func TestResolve_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()

	k := &Kemono{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := k.Resolve(context.Background(), "https://kemono.cr/patreon/user/missing", "")
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

	k := &Kemono{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := k.Resolve(context.Background(), "https://kemono.cr/patreon/user/limited", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestResolve_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/patreon/user/empty/profile":
			if err := json.NewEncoder(w).Encode(profileResponse{ID: "empty", Name: "empty"}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "/api/v1/patreon/user/empty/posts":
			if err := json.NewEncoder(w).Encode([]postResponse{}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	k := &Kemono{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := k.Resolve(context.Background(), "https://kemono.cr/patreon/user/empty", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDownloadRequest_NoLink(t *testing.T) {
	k := &Kemono{}
	_, err := k.DownloadRequest(context.Background(), site.File{ID: "missing", Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestName(t *testing.T) {
	k := &Kemono{}
	if k.Name() != "kemono" {
		t.Errorf("Name() = %q, want kemono", k.Name())
	}
}

func TestDefaults(t *testing.T) {
	k := &Kemono{}
	if k.DefaultConcurrency() != 3 {
		t.Errorf("DefaultConcurrency() = %d, want 3", k.DefaultConcurrency())
	}
	if k.DefaultResolveDelay() != time.Second {
		t.Errorf("DefaultResolveDelay() = %v, want 1s", k.DefaultResolveDelay())
	}
	if k.DefaultDownloadDelay() != 500*time.Millisecond {
		t.Errorf("DefaultDownloadDelay() = %v, want 500ms", k.DefaultDownloadDelay())
	}
}
