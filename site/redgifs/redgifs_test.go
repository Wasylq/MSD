package redgifs

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
	r := &Redgifs{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.redgifs.com/users/sweetiefox", true},
		{"https://redgifs.com/users/sweetiefox", true},
		{"https://www.redgifs.com/niches/just-boobs", true},
		{"https://www.redgifs.com/watch/example", false},
		{"https://example.com/users/sweetiefox", false},
		{"not a url", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := r.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolve_UserWithPagination(t *testing.T) {
	var tokenRequests, pageRequests int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/auth/temporary":
			tokenRequests++
			writeJSON(t, w, map[string]any{"token": "token1"})
		case "/v2/users/sweetiefox/search":
			pageRequests++
			if got := r.Header.Get("Authorization"); got != "Bearer token1" {
				t.Errorf("Authorization = %q", got)
			}
			if got := r.URL.Query().Get("count"); got != "100" {
				t.Errorf("count = %q", got)
			}
			if got := r.URL.Query().Get("order"); got != "new" {
				t.Errorf("order = %q", got)
			}
			switch r.URL.Query().Get("page") {
			case "1":
				writeJSON(t, w, listingResponse{
					Page:  1,
					Pages: 2,
					Gifs: []gifResponse{
						{
							ID:         "firstgif",
							CreateDate: 1704153600,
							UserName:   "sweetiefox",
							URLs: gifURLs{
								HD: "https://media.redgifs.com/Firstgif.mp4",
								SD: "https://media.redgifs.com/Firstgif-mobile.mp4",
							},
						},
					},
				})
			case "2":
				writeJSON(t, w, listingResponse{
					Page:  2,
					Pages: 2,
					Gifs: []gifResponse{
						{
							ID:         "secondgif",
							CreateDate: 1704240000,
							UserName:   "sweetiefox",
							URLs: gifURLs{
								SD: "https://media.redgifs.com/Secondgif-mobile.mp4",
							},
						},
						{
							ID:         "firstgif",
							CreateDate: 1704153600,
							UserName:   "sweetiefox",
							URLs: gifURLs{
								HD: "https://media.redgifs.com/Firstgif.mp4",
							},
						},
					},
				})
			default:
				t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	r := &Redgifs{HTTPClient: ts.Client(), BaseURL: ts.URL, APIBaseURL: ts.URL}
	album, err := r.Resolve(context.Background(), "https://www.redgifs.com/users/sweetiefox", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if tokenRequests != 1 {
		t.Errorf("token requests = %d, want 1", tokenRequests)
	}
	if pageRequests != 2 {
		t.Errorf("page requests = %d, want 2", pageRequests)
	}
	if album.ID != "user-sweetiefox" {
		t.Errorf("album ID = %q, want user-sweetiefox", album.ID)
	}
	if album.Name != "redgifs-user-sweetiefox" {
		t.Errorf("album Name = %q, want redgifs-user-sweetiefox", album.Name)
	}
	if len(album.Files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(album.Files))
	}
	if album.Files[0].ID != "https://media.redgifs.com/Firstgif.mp4" {
		t.Errorf("first download URL = %q", album.Files[0].ID)
	}
	if album.Files[0].Name != "20240102_sweetiefox_firstgif.mp4" {
		t.Errorf("first file name = %q", album.Files[0].Name)
	}
	if album.Files[1].Name != "20240103_sweetiefox_secondgif.mp4" {
		t.Errorf("second file name = %q", album.Files[1].Name)
	}
	if len(album.PostLinks) != 2 || album.PostLinks[0] != ts.URL+"/watch/firstgif" {
		t.Errorf("PostLinks = %#v", album.PostLinks)
	}
}

func TestResolve_Niche(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/auth/temporary":
			writeJSON(t, w, map[string]any{"token": "token1"})
		case "/v2/niches/just-boobs/gifs":
			writeJSON(t, w, listingResponse{
				Page:  1,
				Pages: 1,
				Gifs: []gifResponse{
					{
						ID:         "nichegif",
						CreateDate: 1704153600,
						UserName:   "creator",
						URLs: gifURLs{
							HD: "https://media.redgifs.com/Nichegif.mp4",
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	r := &Redgifs{HTTPClient: ts.Client(), BaseURL: ts.URL, APIBaseURL: ts.URL}
	album, err := r.Resolve(context.Background(), "https://www.redgifs.com/niches/just-boobs", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if album.ID != "niche-just-boobs" {
		t.Errorf("album ID = %q", album.ID)
	}
	if album.Name != "redgifs-niche-just-boobs" {
		t.Errorf("album Name = %q", album.Name)
	}
	if len(album.Files) != 1 || album.Files[0].Name != "20240102_creator_nichegif.mp4" {
		t.Errorf("Files = %#v", album.Files)
	}
}

func TestResolve_RateLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	r := &Redgifs{HTTPClient: ts.Client(), APIBaseURL: ts.URL}
	_, err := r.Resolve(context.Background(), "https://www.redgifs.com/users/sweetiefox", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsRateLimited(err) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestDownloadRequest(t *testing.T) {
	r := &Redgifs{}
	req, err := r.DownloadRequest(context.Background(), site.File{ID: "https://media.redgifs.com/Test.mp4"})
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if req.URL != "https://media.redgifs.com/Test.mp4" {
		t.Errorf("URL = %q", req.URL)
	}
	if req.Headers.Get("User-Agent") == "" {
		t.Error("User-Agent header is empty")
	}
	if req.Headers.Get("Referer") != defaultBaseURL+"/" {
		t.Errorf("Referer = %q", req.Headers.Get("Referer"))
	}
}

func TestDownloadRequestRejectsBadURL(t *testing.T) {
	r := &Redgifs{}
	_, err := r.DownloadRequest(context.Background(), site.File{ID: "not a url"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseRedgifsURLKeepsEscapedID(t *testing.T) {
	rawURL := "https://www.redgifs.com/niches/" + url.PathEscape("just-boobs")
	ref := parseRedgifsURL(rawURL)
	if ref == nil {
		t.Fatal("parseRedgifsURL returned nil")
	}
	if ref.kind != "niche" || ref.id != "just-boobs" {
		t.Errorf("ref = %#v", ref)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
