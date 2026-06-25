package coomerfans

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wasylq/MSD/site"
)

func TestMatch(t *testing.T) {
	c := &CoomerFans{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://coomerfans.com/u/onlyfans/369641/u211468182", true},
		{"https://coomerfans.com/u/fansly/322248/lilc0smic?page=2", true},
		{"https://coomerfans.com/p/83817107/369641/onlyfans", true},
		{"https://coomerfans.com/u/onlyfans/369641", false},
		{"https://coomerfans.com/random-post", false},
		{"https://other.com/u/onlyfans/369641/u211468182", false},
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

func TestResolveCreator(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected User-Agent")
		}

		switch {
		case r.URL.Path == "/u/onlyfans/369641/u211468182" && r.URL.Query().Get("page") == "":
			_, _ = w.Write([]byte(creatorPage1))
		case r.URL.Path == "/u/onlyfans/369641/u211468182" && r.URL.Query().Get("page") == "2":
			_, _ = w.Write([]byte(creatorPage2))
		case r.URL.Path == "/p/83817107/369641/onlyfans":
			_, _ = w.Write([]byte(postPageImage))
		case r.URL.Path == "/p/83817119/369641/onlyfans":
			_, _ = w.Write([]byte(postPageVideo))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := &CoomerFans{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := c.Resolve(context.Background(), "https://coomerfans.com/u/onlyfans/369641/u211468182", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.ID != "onlyfans-369641" {
		t.Errorf("album ID = %q, want onlyfans-369641", album.ID)
	}
	if album.Name != "coomerfans-onlyfans-u211468182" {
		t.Errorf("album Name = %q", album.Name)
	}
	if len(album.PostLinks) != 2 {
		t.Fatalf("len(PostLinks) = %d, want 2", len(album.PostLinks))
	}
	if album.PostLinks[0] != "https://coomerfans.com/p/83817107/369641/onlyfans" {
		t.Errorf("PostLinks[0] = %q", album.PostLinks[0])
	}
	if len(album.Files) != 3 {
		t.Fatalf("len(Files) = %d, want 3", len(album.Files))
	}

	if album.Files[0].ID != "https://img1.coomerfans.com/storage/2/sy/fa/hash.jpg" {
		t.Errorf("file[0].ID = %q", album.Files[0].ID)
	}
	if !strings.Contains(album.Files[0].Name, "2025-03-05 - Sample Post - 83817107 - 01 - hash.jpg") {
		t.Errorf("file[0].Name = %q", album.Files[0].Name)
	}
	if album.Files[2].ID != "https://img2.coomerfans.com/storage/2/aa/bb/movie.mp4" {
		t.Errorf("file[2].ID = %q", album.Files[2].ID)
	}
	if album.Files[0].Size != -1 {
		t.Errorf("file[0].Size = %d, want -1", album.Files[0].Size)
	}
}

func TestResolvePost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/p/83817107/369641/onlyfans" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(postPageImage))
	}))
	defer ts.Close()

	c := &CoomerFans{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := c.Resolve(context.Background(), "https://coomerfans.com/p/83817107/369641/onlyfans", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.ID != "83817107" {
		t.Errorf("album ID = %q", album.ID)
	}
	if len(album.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(album.Files))
	}
}

func TestResolve_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()

	c := &CoomerFans{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := c.Resolve(context.Background(), "https://coomerfans.com/u/onlyfans/369641/u211468182", "")
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

	c := &CoomerFans{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := c.Resolve(context.Background(), "https://coomerfans.com/p/83817107/369641/onlyfans", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestResolve_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><div class="posts-list"></div></body></html>`))
	}))
	defer ts.Close()

	c := &CoomerFans{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := c.Resolve(context.Background(), "https://coomerfans.com/u/onlyfans/369641/u211468182", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDownloadRequest(t *testing.T) {
	c := &CoomerFans{}
	req, err := c.DownloadRequest(context.Background(), site.File{
		ID:   "https://img1.coomerfans.com/storage/2/sy/fa/hash.jpg",
		Name: "hash.jpg",
	})
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if req.URL != "https://img1.coomerfans.com/storage/2/sy/fa/hash.jpg" {
		t.Errorf("URL = %q", req.URL)
	}
	if req.Headers.Get("Referer") != defaultBaseURL+"/" {
		t.Errorf("Referer = %q", req.Headers.Get("Referer"))
	}
}

func TestDownloadRequest_NoLink(t *testing.T) {
	c := &CoomerFans{}
	_, err := c.DownloadRequest(context.Background(), site.File{ID: "not-a-url", Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestName(t *testing.T) {
	c := &CoomerFans{}
	if c.Name() != "coomerfans" {
		t.Errorf("Name() = %q", c.Name())
	}
}

func TestDefaults(t *testing.T) {
	c := &CoomerFans{}
	if c.DefaultConcurrency() != 3 {
		t.Errorf("DefaultConcurrency() = %d, want 3", c.DefaultConcurrency())
	}
	if c.DefaultResolveDelay() != time.Second {
		t.Errorf("DefaultResolveDelay() = %v, want 1s", c.DefaultResolveDelay())
	}
	if c.DefaultDownloadDelay() != 500*time.Millisecond {
		t.Errorf("DefaultDownloadDelay() = %v, want 500ms", c.DefaultDownloadDelay())
	}
}

const creatorPage1 = `
<html><body>
<article class="model-info"><h1>u211468182</h1></article>
<div class="posts-list">
  <div class="post">
    <h3><a href="/p/83817107/369641/onlyfans">Sample Post</a></h3>
    <span class="post-date">2025-03-05 11:46:07 +0000 UTC</span>
    <a class="view-post" href="/p/83817107/369641/onlyfans">View Post</a>
  </div>
</div>
<div class="pagination"><a class="next" href="/u/onlyfans/369641/u211468182?page=2">Next</a></div>
</body></html>`

const creatorPage2 = `
<html><body>
<div class="posts-list">
  <div class="post">
    <h3><a href="/p/83817119/369641/onlyfans">Video Post</a></h3>
    <span class="post-date">2025-01-28 15:37:19 +0000 UTC</span>
    <a class="view-post" href="/p/83817119/369641/onlyfans">View Post</a>
  </div>
</div>
</body></html>`

const postPageImage = `
<html><body>
<div class="post-wrap">
  <h1>Sample Post</h1>
  <span class="post-date">Added 2025-03-05 11:46:07 +0000 UTC</span>
  <div class="post-body">
    <img src="https://img1.coomerfans.com/storage/2/sy/fa/hash.jpg" alt="">
    <img src="https://img1.coomerfans.com/storage/2/sy/fa/hash.jpg" alt="">
    <img src="https://other.test/storage/2/sy/fa/ignore.jpg" alt="">
  </div>
</div>
</body></html>`

const postPageVideo = `
<html><body>
<div class="post-wrap">
  <h1>Video Post</h1>
  <span class="post-date">Added 2025-01-28 15:37:19 +0000 UTC</span>
  <div class="post-body">
    <video src="https://img1.coomerfans.com/storage/2/aa/bb/clip.mp4"></video>
    <video><source src="https://img2.coomerfans.com/storage/2/aa/bb/movie.mp4"></video>
  </div>
</div>
</body></html>`
