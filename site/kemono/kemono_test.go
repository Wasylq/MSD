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
		{"https://kemono.cr/patreon/user/59577203", true},
		{"https://kemono.cr/fanbox/user/abc_123", true},
		{"https://kemono.cr/patreon/user/59577203?o=50", true},
		{"https://pawchive.st/patreon/user/59577203", true},
		{"https://pawchive.st/patreon/user/59577203?o=50", true},
		{"https://kemono.cr/patreon/post/123", false},
		{"https://other.com/patreon/user/59577203", false},
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
	userURL, err := parseUserURL("https://pawchive.st/patreon/user/59577203")
	if err != nil {
		t.Fatalf("parseUserURL: %v", err)
	}

	got := k.dataURL("/24/0e/hash.png", "Rosa 1.png", userURL)
	want := "https://file.pawchive.st/data/24/0e/hash.png?f=Rosa+1.png"
	if got != want {
		t.Errorf("data URL = %q, want %q", got, want)
	}

	k.UseThumbnails = true
	got = k.dataURL("/24/0e/hash.png", "Rosa 1.png", userURL)
	want = "https://img.pawchive.st/thumbnail/data/24/0e/hash.png"
	if got != want {
		t.Errorf("thumbnail URL = %q, want %q", got, want)
	}

	got = postURL(userURL, "161451659")
	want = "https://pawchive.st/patreon/user/59577203/post/161451659"
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
		case "/api/v1/patreon/user/59577203/profile":
			json.NewEncoder(w).Encode(profileResponse{
				ID:      "59577203",
				Name:    "pizzacakecomic",
				Service: "patreon",
			})
		case "/api/v1/patreon/user/59577203/posts":
			if r.URL.Query().Get("o") != "0" {
				json.NewEncoder(w).Encode([]postResponse{})
				return
			}
			json.NewEncoder(w).Encode([]postResponse{
				{
					ID:        "153061906",
					User:      "59577203",
					Service:   "patreon",
					Title:     "Flowy",
					Published: "2026-03-18T16:00:12",
					File: kemonoFile{
						Name: "Comic-flowy.png",
						Path: "/9e/fa/hash.png",
					},
					Attachments: []kemonoFile{
						{Name: "Comic-flowy.png", Path: "/9e/fa/hash.png"},
						{Name: "Extra.png", Path: "/aa/bb/extra.png"},
					},
				},
				{
					ID:          "145296000",
					User:        "59577203",
					Service:     "patreon",
					Title:       "Text only",
					Published:   "2025-12-07T18:26:55",
					Attachments: []kemonoFile{},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	k := &Kemono{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := k.Resolve(context.Background(), "https://kemono.cr/patreon/user/59577203", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.ID != "patreon-59577203" {
		t.Errorf("album ID = %q, want patreon-59577203", album.ID)
	}
	if album.Name != "kemono-patreon-pizzacakecomic" {
		t.Errorf("album Name = %q, want kemono-patreon-pizzacakecomic", album.Name)
	}
	if len(album.Files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(album.Files))
	}
	wantPostLinks := []string{
		"https://kemono.cr/patreon/user/59577203/post/153061906",
		"https://kemono.cr/patreon/user/59577203/post/145296000",
	}
	if len(album.PostLinks) != len(wantPostLinks) {
		t.Fatalf("len(PostLinks) = %d, want %d", len(album.PostLinks), len(wantPostLinks))
	}
	for i, want := range wantPostLinks {
		if album.PostLinks[i] != want {
			t.Errorf("PostLinks[%d] = %q, want %q", i, album.PostLinks[i], want)
		}
	}

	if album.Files[0].ID != "153061906:/9e/fa/hash.png" {
		t.Errorf("file[0].ID = %q", album.Files[0].ID)
	}
	if !strings.Contains(album.Files[0].Name, "Flowy") {
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
		case "/api/v1/patreon/user/59577203/profile":
			json.NewEncoder(w).Encode(profileResponse{ID: "59577203", Name: "pizzacakecomic"})
		case "/api/v1/patreon/user/59577203/posts":
			json.NewEncoder(w).Encode([]postResponse{
				{
					ID:        "153061906",
					Title:     "Flowy",
					Published: "2026-03-18T16:00:12",
					Attachments: []kemonoFile{
						{Name: "Comic-flowy.png", Path: "/9e/fa/hash.png"},
					},
				},
			})
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
	album, err := k.Resolve(context.Background(), "https://kemono.cr/patreon/user/59577203", "")
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
			json.NewEncoder(w).Encode(profileResponse{ID: "empty", Name: "empty"})
		case "/api/v1/patreon/user/empty/posts":
			json.NewEncoder(w).Encode([]postResponse{})
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
