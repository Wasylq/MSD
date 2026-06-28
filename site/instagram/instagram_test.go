package instagram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wasylq/MSD/site"
)

func TestMatch(t *testing.T) {
	i := &Instagram{}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.instagram.com/salmahayek/", true},
		{"https://instagram.com/salmahayek", true},
		{"https://www.instagram.com/p/ABC123/", false},
		{"https://www.instagram.com/reel/ABC123/", false},
		{"https://example.com/salmahayek/", false},
		{"not a url", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := i.Match(tt.url); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolve_ProfileWithPagination(t *testing.T) {
	var sawProfile, sawTimeline bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case profileInfoPath:
			sawProfile = true
			if got := r.URL.Query().Get("username"); got != "salmahayek" {
				t.Errorf("profile username = %q", got)
			}
			if got := r.Header.Get("X-IG-App-ID"); got != webAppID {
				t.Errorf("X-IG-App-ID = %q", got)
			}
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"data": map[string]any{
					"user": map[string]any{
						"id":         "123",
						"username":   "salmahayek",
						"is_private": false,
						"edge_owner_to_timeline_media": map[string]any{
							"count": 3,
							"page_info": map[string]any{
								"has_next_page": true,
								"end_cursor":    "cursor1",
							},
							"edges": []any{
								mediaEdgeJSON("p1", "ABC1", "GraphImage", 1704153600, "https://cdn.example/photo1.jpg", ""),
								map[string]any{
									"node": map[string]any{
										"__typename":         "GraphSidecar",
										"id":                 "p2",
										"shortcode":          "ABC2",
										"taken_at_timestamp": float64(1704153600),
										"edge_sidecar_to_children": map[string]any{
											"edges": []any{
												mediaEdgeJSON("c1", "", "GraphImage", 0, "https://cdn.example/photo2.jpg", ""),
												mediaEdgeJSON("c2", "", "GraphVideo", 0, "", "https://cdn.example/video1.mp4"),
											},
										},
									},
								},
							},
						},
					},
				},
			})
		case "/graphql/query/":
			sawTimeline = true
			if got := r.URL.Query().Get("query_id"); got != timelineQueryID {
				t.Errorf("query_id = %q", got)
			}
			var variables map[string]any
			if err := json.Unmarshal([]byte(r.URL.Query().Get("variables")), &variables); err != nil {
				t.Fatalf("decode variables: %v", err)
			}
			if variables["id"] != "123" || variables["after"] != "cursor1" {
				t.Errorf("variables = %#v", variables)
			}
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"data": map[string]any{
					"user": map[string]any{
						"edge_owner_to_timeline_media": map[string]any{
							"count": 3,
							"page_info": map[string]any{
								"has_next_page": false,
								"end_cursor":    "",
							},
							"edges": []any{
								mediaEdgeJSON("p3", "ABC3", "GraphVideo", 1704240000, "", "https://cdn.example/video2.mp4"),
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	i := &Instagram{HTTPClient: ts.Client(), BaseURL: ts.URL}
	album, err := i.Resolve(context.Background(), "https://www.instagram.com/salmahayek/", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !sawProfile || !sawTimeline {
		t.Fatalf("expected profile and timeline endpoints to be called")
	}
	if album.ID != "123" {
		t.Errorf("album ID = %q, want 123", album.ID)
	}
	if album.Name != "salmahayek" {
		t.Errorf("album Name = %q, want salmahayek", album.Name)
	}

	wantNames := []string{"240102_1.jpg", "240102_2.jpg", "240102_3.mp4", "240103_1.mp4"}
	if len(album.Files) != len(wantNames) {
		t.Fatalf("len(files) = %d, want %d", len(album.Files), len(wantNames))
	}
	for idx, want := range wantNames {
		if album.Files[idx].Name != want {
			t.Errorf("file[%d].Name = %q, want %q", idx, album.Files[idx].Name, want)
		}
	}
	if len(album.PostLinks) != 3 {
		t.Fatalf("len(PostLinks) = %d, want 3", len(album.PostLinks))
	}
	if album.PostLinks[0] != ts.URL+"/p/ABC1/" {
		t.Errorf("first post link = %q", album.PostLinks[0])
	}

	req, err := i.DownloadRequest(context.Background(), album.Files[2])
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if req.URL != "https://cdn.example/video1.mp4" {
		t.Errorf("download URL = %q", req.URL)
	}
}

func TestResolve_PrivateProfile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"status": "ok",
			"data": map[string]any{
				"user": map[string]any{
					"id":         "123",
					"username":   "salmahayek",
					"is_private": true,
				},
			},
		})
	}))
	defer ts.Close()

	i := &Instagram{HTTPClient: ts.Client(), BaseURL: ts.URL}
	_, err := i.Resolve(context.Background(), "https://www.instagram.com/salmahayek/", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !site.IsAuthRequired(err) {
		t.Errorf("expected ErrAuthRequired, got %v", err)
	}
}

func TestPostDate(t *testing.T) {
	ts := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC).Unix()
	if got := postDate(ts); got != "240102" {
		t.Errorf("postDate = %q, want 240102", got)
	}
}

func mediaEdgeJSON(id, shortcode, typename string, takenAt int64, imageURL, videoURL string) map[string]any {
	node := map[string]any{
		"__typename": typename,
		"id":         id,
	}
	if shortcode != "" {
		node["shortcode"] = shortcode
	}
	if takenAt != 0 {
		node["taken_at_timestamp"] = float64(takenAt)
	}
	if imageURL != "" {
		node["display_url"] = imageURL
	}
	if videoURL != "" {
		node["is_video"] = true
		node["video_url"] = videoURL
	}
	return map[string]any{"node": node}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func TestParseProfileURLRejectsReservedPaths(t *testing.T) {
	for _, rawURL := range []string{
		"https://www.instagram.com/p/ABC/",
		"https://www.instagram.com/reel/ABC/",
		"https://www.instagram.com/stories/salmahayek/",
	} {
		if got := parseProfileURL(rawURL); got != "" {
			t.Errorf("parseProfileURL(%q) = %q", rawURL, got)
		}
	}
}

func TestBestResourceURL(t *testing.T) {
	got := bestResourceURL([]imageResource{
		{URL: "https://cdn.example/small.jpg", Width: 100, Height: 100},
		{URL: "https://cdn.example/large.jpg", Width: 200, Height: 100},
	})
	if got != "https://cdn.example/large.jpg" {
		t.Errorf("bestResourceURL = %q", got)
	}
}

func TestExtensionFallsBackByMediaType(t *testing.T) {
	image := mediaNode{DisplayURL: "https://cdn.example/image?id=1"}
	if got := image.extension(); got != ".jpg" {
		t.Errorf("image extension = %q", got)
	}
	video := mediaNode{IsVideo: true, VideoURL: "https://cdn.example/video?id=1"}
	if got := video.extension(); got != ".mp4" {
		t.Errorf("video extension = %q", got)
	}
}

func TestProfileURLKeepsEscapedUsername(t *testing.T) {
	username := "salmahayek"
	raw := "https://www.instagram.com/" + url.PathEscape(username) + "/"
	if got := parseProfileURL(raw); got != username {
		t.Errorf("parseProfileURL = %q", got)
	}
}

func TestDownloadRequestMissingLink(t *testing.T) {
	i := &Instagram{}
	_, err := i.DownloadRequest(context.Background(), site.File{ID: "missing"})
	if err == nil || !strings.Contains(err.Error(), "no download link") {
		t.Fatalf("expected missing link error, got %v", err)
	}
}
