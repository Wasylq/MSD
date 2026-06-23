package kemono

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Wasylq/MSD/site"
)

const (
	defaultBaseURL      = "https://kemono.cr"
	defaultThumbnailURL = "https://img.kemono.cr"
	userAgent           = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
	pageSize            = 50
)

var supportedHosts = map[string]struct{}{
	"kemono.cr":   {},
	"pawchive.st": {},
}

func init() { site.Register(&Kemono{}) }

type Kemono struct {
	HTTPClient       *http.Client
	BaseURL          string
	ThumbnailBaseURL string
	UseThumbnails    bool

	mu    sync.Mutex
	links map[string]string
	base  string
}

func (k *Kemono) Name() string { return "kemono" }

func (k *Kemono) Match(rawURL string) bool {
	_, err := parseUserURL(rawURL)
	return err == nil
}

func (k *Kemono) Resolve(ctx context.Context, rawURL string, _ string) (*site.Album, error) {
	userURL, err := parseUserURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("kemono: %w: %s", site.ErrNotFound, rawURL)
	}
	baseURL := k.baseURL(userURL)
	service, creatorID := userURL.service, userURL.creatorID

	profile, err := k.fetchProfile(ctx, baseURL, service, creatorID)
	if err != nil {
		return nil, err
	}

	k.mu.Lock()
	k.links = make(map[string]string)
	k.base = baseURL
	k.mu.Unlock()

	album := &site.Album{
		ID:   service + "-" + creatorID,
		Name: albumName(service, creatorID, profile.Name),
	}

	for offset := 0; ; offset += pageSize {
		posts, err := k.fetchPosts(ctx, baseURL, service, creatorID, offset)
		if err != nil {
			return nil, err
		}
		for _, post := range posts {
			if post.ID != "" {
				album.PostLinks = append(album.PostLinks, postURL(userURL, post.ID))
			}
			k.addPostFiles(album, post, userURL)
		}
		if len(posts) < pageSize {
			break
		}
	}

	if len(album.Files) == 0 {
		return nil, fmt.Errorf("kemono: %w: no downloadable files for %s/%s", site.ErrNotFound, service, creatorID)
	}

	return album, nil
}

func (k *Kemono) fetchProfile(ctx context.Context, baseURL, service, creatorID string) (*profileResponse, error) {
	var profile profileResponse
	if err := k.getJSON(ctx, baseURL, fmt.Sprintf("/api/v1/%s/user/%s/profile", service, creatorID), &profile); err != nil {
		return nil, err
	}
	if profile.ID == "" {
		return nil, fmt.Errorf("kemono: %w: %s/%s", site.ErrNotFound, service, creatorID)
	}
	return &profile, nil
}

func (k *Kemono) fetchPosts(ctx context.Context, baseURL, service, creatorID string, offset int) ([]postResponse, error) {
	var posts []postResponse
	apiPath := fmt.Sprintf("/api/v1/%s/user/%s/posts?o=%d", service, creatorID, offset)
	if err := k.getJSON(ctx, baseURL, apiPath, &posts); err != nil {
		return nil, err
	}
	return posts, nil
}

func (k *Kemono) getJSON(ctx context.Context, baseURL, apiPath string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+apiPath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/css")
	req.Header.Set("Referer", baseURL+"/")

	resp, err := k.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return fmt.Errorf("kemono: %w: %s", site.ErrNotFound, apiPath)
	case http.StatusTooManyRequests:
		return fmt.Errorf("kemono: %w", site.ErrRateLimited)
	case http.StatusForbidden:
		return fmt.Errorf("kemono: %w: API forbidden", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return fmt.Errorf("kemono: unexpected status %d for %s", resp.StatusCode, apiPath)
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("kemono: decode response: %w", err)
	}
	return nil
}

func (k *Kemono) addPostFiles(album *site.Album, post postResponse, userURL *userURL) {
	seen := make(map[string]struct{})
	files := append([]kemonoFile(nil), post.Attachments...)
	if post.File.Path != "" || post.File.Name != "" {
		files = append(files, post.File)
	}

	for _, f := range files {
		if f.Path == "" {
			continue
		}
		if _, ok := seen[f.Path]; ok {
			continue
		}
		seen[f.Path] = struct{}{}

		name := f.Name
		if name == "" {
			name = path.Base(f.Path)
		}

		index := len(seen)
		id := post.ID + ":" + f.Path
		fileName := postFileName(post, index, name)
		link := k.dataURL(f.Path, name, userURL)

		k.mu.Lock()
		k.links[id] = link
		k.mu.Unlock()

		album.Files = append(album.Files, site.File{
			ID:   id,
			Name: fileName,
			Size: -1,
		})
	}
}

func (k *Kemono) DownloadRequest(_ context.Context, file site.File) (*site.DownloadRequest, error) {
	k.mu.Lock()
	link := k.links[file.ID]
	baseURL := k.base
	k.mu.Unlock()
	if link == "" {
		return nil, fmt.Errorf("kemono: no download link for %s", file.ID)
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &site.DownloadRequest{
		URL: link,
		Headers: http.Header{
			"User-Agent": {userAgent},
			"Referer":    {baseURL + "/"},
		},
	}, nil
}

func (k *Kemono) DefaultConcurrency() int             { return 3 }
func (k *Kemono) DefaultResolveDelay() time.Duration  { return time.Second }
func (k *Kemono) DefaultDownloadDelay() time.Duration { return 500 * time.Millisecond }

func (k *Kemono) httpClient() *http.Client {
	if k.HTTPClient != nil {
		return k.HTTPClient
	}
	return http.DefaultClient
}

func (k *Kemono) baseURL(userURL *userURL) string {
	if k.BaseURL != "" {
		return strings.TrimRight(k.BaseURL, "/")
	}
	if userURL != nil && userURL.origin != "" {
		return userURL.origin
	}
	return defaultBaseURL
}

func (k *Kemono) dataURL(filePath, name string, userURL *userURL) string {
	filePath = "/" + strings.TrimLeft(filePath, "/")
	if k.UseThumbnails {
		return k.thumbnailBaseURL(userURL) + "/thumbnail/data" + filePath
	}
	link := k.dataBaseURL(userURL) + "/data" + filePath
	if k.BaseURL == "" && userURL != nil && userURL.host == "pawchive.st" && name != "" {
		link += "?f=" + url.QueryEscape(name)
	}
	return link
}

func (k *Kemono) dataBaseURL(userURL *userURL) string {
	if k.BaseURL != "" {
		return strings.TrimRight(k.BaseURL, "/")
	}
	if userURL != nil && userURL.host == "pawchive.st" {
		return "https://file.pawchive.st"
	}
	return k.baseURL(userURL)
}

func (k *Kemono) thumbnailBaseURL(userURL *userURL) string {
	if k.ThumbnailBaseURL != "" {
		return strings.TrimRight(k.ThumbnailBaseURL, "/")
	}
	if k.BaseURL != "" {
		return strings.TrimRight(k.BaseURL, "/")
	}
	if userURL != nil && userURL.host == "pawchive.st" {
		return "https://img.pawchive.st"
	}
	return defaultThumbnailURL
}

func parseUserURL(rawURL string) (*userURL, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid URL")
	}

	host := strings.ToLower(u.Hostname())
	if _, ok := supportedHosts[host]; !ok {
		return nil, fmt.Errorf("unsupported host")
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] != "user" || parts[2] == "" {
		return nil, fmt.Errorf("invalid user URL")
	}

	return &userURL{
		service:   parts[0],
		creatorID: parts[2],
		host:      host,
		origin:    u.Scheme + "://" + u.Host,
	}, nil
}

func postURL(userURL *userURL, postID string) string {
	return userURL.origin + "/" + userURL.service + "/user/" + userURL.creatorID + "/post/" + postID
}

func albumName(service, creatorID, name string) string {
	if name == "" {
		name = creatorID
	}
	return "kemono-" + service + "-" + name
}

func postFileName(post postResponse, index int, name string) string {
	parts := []string{}
	if len(post.Published) >= len("2006-01-02") {
		parts = append(parts, post.Published[:len("2006-01-02")])
	}
	if post.Title != "" {
		parts = append(parts, post.Title)
	}
	if post.ID != "" {
		parts = append(parts, post.ID)
	}
	parts = append(parts, fmt.Sprintf("%02d", index), name)
	return strings.Join(parts, " - ")
}

type profileResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Service string `json:"service"`
}

type postResponse struct {
	ID          string       `json:"id"`
	User        string       `json:"user"`
	Service     string       `json:"service"`
	Title       string       `json:"title"`
	Published   string       `json:"published"`
	File        kemonoFile   `json:"file"`
	Attachments []kemonoFile `json:"attachments"`
}

type kemonoFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type userURL struct {
	service   string
	creatorID string
	host      string
	origin    string
}
