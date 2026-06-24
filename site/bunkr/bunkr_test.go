package bunkr

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
	b := &Bunkr{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://bunkr.cr/a/Z64Hzaqy", true},
		{"https://bunkr.cr/f/ukoESVUMmJ0Fd", true},
		{"https://bunkr.cr/i/abc123", true},
		{"https://bunkr.cr/v/abc123", true},
		{"https://bunkr-albums.io/a/Z64Hzaqy", true},
		{"https://balbums.st/a/Z64Hzaqy", true},
		{"https://bunkr.cr/a/", false},
		{"https://bunkr.cr/x/abc", false},
		{"https://other.test/a/Z64Hzaqy", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := b.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolveAlbum(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/a/Z64Hzaqy" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("advanced") != "1" {
			t.Errorf("advanced query = %q, want 1", r.URL.Query().Get("advanced"))
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`
			<html>
				<head><title>Christine/ Mysexylegs | Bunkr</title></head>
				<body>
					<div class="text-subs font-semibold flex text-base sm:text-lg">
						<h1 class="truncate">Christine/ Mysexylegs</h1>
					</div>
					<script>
					window.albumFiles = [
					{
					  id:  57087394 ,
					  name: "b00459db-b71a-472e-a89f-b9ba94264355.mp4",
					  original: "34.mp4",
					  slug: "ukoESVUMmJ0Fd",
					  type: "video/mp4",
					  extension: "Video",
					  size:  431905062 ,
					  timestamp: "10:00:17 29/12/2025",
					  thumbnail: "https://static.scdn.st/thumb.png",
					  cdnEndpoint: "/b00459db-b71a-472e-a89f-b9ba94264355.mp4"
					}
					];
					</script>
				</body>
			</html>`))
	}))
	defer ts.Close()

	b := &Bunkr{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := b.Resolve(context.Background(), "https://bunkr.cr/a/Z64Hzaqy", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if album.ID != "Z64Hzaqy" {
		t.Errorf("album ID = %q, want %q", album.ID, "Z64Hzaqy")
	}
	if album.Name != "Christine/ Mysexylegs" {
		t.Errorf("album Name = %q, want %q", album.Name, "Christine/ Mysexylegs")
	}
	if len(album.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(album.Files))
	}
	if album.Files[0].ID != "57087394" {
		t.Errorf("file ID = %q, want %q", album.Files[0].ID, "57087394")
	}
	if album.Files[0].Name != "34.mp4" {
		t.Errorf("file Name = %q, want %q", album.Files[0].Name, "34.mp4")
	}
	if album.Files[0].Size != 431905062 {
		t.Errorf("file Size = %d, want 431905062", album.Files[0].Size)
	}
}

func TestResolveAlbum_Empty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body><h1>Empty</h1></body></html>"))
	}))
	defer ts.Close()

	b := &Bunkr{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := b.Resolve(context.Background(), "https://bunkr.cr/a/empty", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolveFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/f/ukoESVUMmJ0Fd" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`
				<html>
					<head><title>34.mp4 | Bunkr</title></head>
					<body>
					<h1>34.mp4</h1>
					<div id="fileTracker" data-file-id="57087394"></div>
				</body>
			</html>`))
	}))
	defer ts.Close()

	b := &Bunkr{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := b.Resolve(context.Background(), "https://bunkr.cr/f/ukoESVUMmJ0Fd", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(album.Files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(album.Files))
	}
	if album.Files[0].ID != "57087394" {
		t.Errorf("file ID = %q, want 57087394", album.Files[0].ID)
	}
	if album.Files[0].Name != "34.mp4" {
		t.Errorf("file name = %q, want 34.mp4", album.Files[0].Name)
	}
}

func TestDownloadRequest(t *testing.T) {
	ts := bunkrAPIServer(t)
	defer ts.Close()

	b := &Bunkr{
		HTTPClient: ts.Client(),
		BaseURL:    "https://bunkr.cr",
		BridgeURL:  ts.URL + "/api/_001_v2",
		SignURL:    ts.URL + "/sign",
		slugs:      map[string]string{"57087394": "ukoESVUMmJ0Fd"},
	}
	req, err := b.DownloadRequest(context.Background(), site.File{
		ID:   "57087394",
		Name: "34.mp4",
		Size: 431905062,
	})
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}

	got, err := url.Parse(req.URL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	if got.Scheme != "https" || got.Host != "c2ck-b.cdn.cr" || got.Path != "/storage/media/b00459db-b71a-472e-a89f-b9ba94264355.mp4" {
		t.Errorf("download URL = %q", req.URL)
	}
	if got.Query().Get("token") != "signed-token" {
		t.Errorf("token = %q, want signed-token", got.Query().Get("token"))
	}
	if got.Query().Get("ex") != "1782341509" {
		t.Errorf("ex = %q, want 1782341509", got.Query().Get("ex"))
	}
	if got.Query().Get("n") != "34.mp4" {
		t.Errorf("n = %q, want 34.mp4", got.Query().Get("n"))
	}
	if req.Headers.Get("Referer") != "https://bunkr.cr/f/ukoESVUMmJ0Fd" {
		t.Errorf("Referer = %q", req.Headers.Get("Referer"))
	}
}

func TestDownloadRequest_RateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	b := &Bunkr{HTTPClient: ts.Client(), BridgeURL: ts.URL, SignURL: ts.URL + "/sign"}
	_, err := b.DownloadRequest(context.Background(), site.File{ID: "57087394", Name: "34.mp4"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
}

func TestName(t *testing.T) {
	b := &Bunkr{}
	if b.Name() != "bunkr" {
		t.Errorf("Name() = %q, want bunkr", b.Name())
	}
}

func TestDefaults(t *testing.T) {
	b := &Bunkr{}
	if b.DefaultConcurrency() != 2 {
		t.Errorf("DefaultConcurrency() = %d, want 2", b.DefaultConcurrency())
	}
	if b.DefaultResolveDelay() != 5*1e9 {
		t.Errorf("DefaultResolveDelay() = %v, want 5s", b.DefaultResolveDelay())
	}
	if b.DefaultDownloadDelay() != 2*1e9 {
		t.Errorf("DefaultDownloadDelay() = %v, want 2s", b.DefaultDownloadDelay())
	}
}

func bunkrAPIServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/_001_v2":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if body["id"] != "57087394" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if err := json.NewEncoder(w).Encode(bridgeResponse{
				MediaFiles: "https://c2ck-b.cdn.cr",
				Path:       "/storage/media/b00459db-b71a-472e-a89f-b9ba94264355.mp4",
				Original:   "34.mp4",
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "/sign":
			if r.URL.Query().Get("path") != "/storage/media/b00459db-b71a-472e-a89f-b9ba94264355.mp4" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if err := json.NewEncoder(w).Encode(signResponse{
				Token: "signed-token",
				Ex:    1782341509,
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
}
