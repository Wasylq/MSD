package gofile

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
	g := &Gofile{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://gofile.io/d/abc123", true},
		{"https://gofile.io/d/5cXXCq", true},
		{"http://gofile.io/d/Test1", true},
		{"https://gofile.io/u/abc123", false},
		{"https://other.com/d/abc123", false},
		{"https://other.com/?next=https://gofile.io/d/abc123", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := g.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestComputeWebsiteToken(t *testing.T) {
	// Deterministic: same inputs → same output
	now := time.Unix(14400*100, 0) // exact boundary
	token1 := computeWebsiteToken("testtoken", now)
	token2 := computeWebsiteToken("testtoken", now)
	if token1 != token2 {
		t.Error("same inputs should produce same token")
	}
	if len(token1) != 64 {
		t.Errorf("token length = %d, want 64 (sha256 hex)", len(token1))
	}

	// Different account token → different website token
	token3 := computeWebsiteToken("other", now)
	if token1 == token3 {
		t.Error("different account tokens should produce different website tokens")
	}

	// Empty account token works (for initial account creation)
	token4 := computeWebsiteToken("", now)
	if token4 == "" {
		t.Error("empty account token should still produce a website token")
	}
}

func newTestGofile(ts *httptest.Server) *Gofile {
	return &Gofile{
		HTTPClient:   ts.Client(),
		APIURL:       ts.URL,
		accountToken: "test-token",
		links:        make(map[string]string),
	}
}

func TestResolve_Folder(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/contents/testfolder" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(contentsResponse{
			Status: "ok",
			Data: contentsData{
				ID:   "testfolder",
				Name: "My Album",
				Type: "folder",
				Children: map[string]contentsChild{
					"f1": {ID: "f1", Name: "photo.jpg", Type: "file", Size: 1024, Link: "https://store1.gofile.io/download/f1/photo.jpg"},
					"sub": {
						ID:   "sub",
						Name: "Nested",
						Type: "folder",
						Children: map[string]contentsChild{
							"f2": {ID: "f2", Name: "video.mp4", Type: "file", Size: 2048, Link: "https://store1.gofile.io/download/f2/video.mp4"},
						},
					},
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	g := newTestGofile(ts)
	album, err := g.Resolve(context.Background(), "https://gofile.io/d/testfolder", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.Name != "My Album" {
		t.Errorf("album Name = %q, want %q", album.Name, "My Album")
	}
	if len(album.Files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(album.Files))
	}

	// Check links were stored
	g.mu.Lock()
	if g.links["f1"] == "" || g.links["f2"] == "" {
		t.Error("download links should be stored")
	}
	g.mu.Unlock()
}

func TestResolve_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(contentsResponse{Status: "error-notFound"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	g := newTestGofile(ts)
	_, err := g.Resolve(context.Background(), "https://gofile.io/d/missing", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolve_PasswordRequired(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(contentsResponse{Status: "error-passwordRequired"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	g := newTestGofile(ts)
	_, err := g.Resolve(context.Background(), "https://gofile.io/d/locked", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsAuthRequired(err) {
		t.Errorf("expected ErrAuthRequired, got: %v", err)
	}
}

func TestResolve_WithPassword(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pw := r.URL.Query().Get("password")
		if pw == "" {
			if err := json.NewEncoder(w).Encode(contentsResponse{Status: "error-passwordRequired"}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
			return
		}
		// The browser client sends the password value directly.
		if pw != "mypassword" {
			if err := json.NewEncoder(w).Encode(contentsResponse{Status: "error-passwordIncorrect"}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(contentsResponse{
			Status: "ok",
			Data: contentsData{
				ID:   "locked",
				Name: "Secret Album",
				Type: "folder",
				Children: map[string]contentsChild{
					"f1": {ID: "f1", Name: "secret.txt", Type: "file", Size: 100, Link: "https://store1.gofile.io/download/f1/secret.txt"},
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	g := newTestGofile(ts)
	album, err := g.Resolve(context.Background(), "https://gofile.io/d/locked", "mypassword")
	if err != nil {
		t.Fatalf("Resolve with password: %v", err)
	}
	if len(album.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(album.Files))
	}
}

func TestResolve_RateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	g := newTestGofile(ts)
	_, err := g.Resolve(context.Background(), "https://gofile.io/d/limited", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestResolve_NotPremiumIsAuthRequired(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(contentsResponse{Status: "error-notPremium"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	g := newTestGofile(ts)
	_, err := g.Resolve(context.Background(), "https://gofile.io/d/premium", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsAuthRequired(err) {
		t.Errorf("expected ErrAuthRequired, got: %v", err)
	}
}

func TestResolve_BadURL(t *testing.T) {
	g := &Gofile{}
	_, err := g.Resolve(context.Background(), "https://other.com/nope", "")
	if err == nil {
		t.Fatal("expected error for non-matching URL")
	}
}

func TestDownloadRequest(t *testing.T) {
	g := &Gofile{
		accountToken: "mytoken",
		links:        map[string]string{"f1": "https://store1.gofile.io/download/f1/test.txt"},
	}

	req, err := g.DownloadRequest(context.Background(), site.File{ID: "f1", Name: "test.txt", Size: 100})
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}

	if req.URL != "https://store1.gofile.io/download/f1/test.txt" {
		t.Errorf("URL = %q", req.URL)
	}
	if len(req.Cookies) != 1 || req.Cookies[0].Value != "mytoken" {
		t.Error("expected accountToken cookie")
	}
	if req.Headers.Get("User-Agent") == "" {
		t.Error("expected User-Agent header")
	}
}

func TestDownloadRequest_NoLink(t *testing.T) {
	g := &Gofile{
		accountToken: "mytoken",
		links:        make(map[string]string),
	}

	_, err := g.DownloadRequest(context.Background(), site.File{ID: "missing", Name: "x"})
	if err == nil {
		t.Fatal("expected error for missing link")
	}
}

func TestCreateAccount(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/accounts" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"data":   map[string]any{"token": "newtoken123"},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer ts.Close()

	g := &Gofile{HTTPClient: ts.Client(), APIURL: ts.URL}
	token, err := g.createAccount(context.Background())
	if err != nil {
		t.Fatalf("createAccount: %v", err)
	}
	if token != "newtoken123" {
		t.Errorf("token = %q, want %q", token, "newtoken123")
	}
}

func TestEnsureAccount_UsesEnvToken(t *testing.T) {
	t.Setenv("MSD_GOFILE_TOKEN", "env-token")

	g := &Gofile{}
	if err := g.ensureAccount(context.Background()); err != nil {
		t.Fatalf("ensureAccount: %v", err)
	}
	if g.accountToken != "env-token" {
		t.Errorf("accountToken = %q, want env-token", g.accountToken)
	}
}

func TestName(t *testing.T) {
	g := &Gofile{}
	if g.Name() != "gofile" {
		t.Errorf("Name() = %q", g.Name())
	}
}

func TestDefaults(t *testing.T) {
	g := &Gofile{}
	if g.DefaultConcurrency() != 2 {
		t.Errorf("DefaultConcurrency() = %d, want 2", g.DefaultConcurrency())
	}
	if g.DefaultResolveDelay() != 10*time.Second {
		t.Errorf("DefaultResolveDelay() = %v, want 10s", g.DefaultResolveDelay())
	}
	if g.DefaultDownloadDelay() != 10*time.Second {
		t.Errorf("DefaultDownloadDelay() = %v, want 10s", g.DefaultDownloadDelay())
	}
}
