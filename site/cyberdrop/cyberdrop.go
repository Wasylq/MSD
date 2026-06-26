package cyberdrop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wasylq/MSD/site"
)

const (
	defaultBaseURL = "https://cyberdrop.cr"
	defaultAPIURL  = "https://api.cyberdrop.cr"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
)

func init() { site.Register(&Cyberdrop{}) }

type Cyberdrop struct {
	HTTPClient *http.Client
	BaseURL    string
	APIURL     string
}

func (c *Cyberdrop) Name() string { return "cyberdrop" }

func (c *Cyberdrop) Match(rawURL string) bool {
	_, ok := parseFileSlug(rawURL)
	return ok
}

func (c *Cyberdrop) Resolve(ctx context.Context, rawURL string, _ string) (*site.Album, error) {
	slug, ok := parseFileSlug(rawURL)
	if !ok {
		return nil, fmt.Errorf("cyberdrop: %w: %s", site.ErrNotFound, rawURL)
	}

	info, err := c.fetchInfo(ctx, slug)
	if err != nil {
		return nil, err
	}
	if info.Slug == "" {
		info.Slug = slug
	}

	return &site.Album{
		ID: info.Slug,
		Files: []site.File{
			{
				ID:   info.Slug,
				Name: info.Name,
				Size: info.Size,
			},
		},
	}, nil
}

func (c *Cyberdrop) DownloadRequest(ctx context.Context, file site.File) (*site.DownloadRequest, error) {
	signedURL, err := c.fetchSignedURL(ctx, file.ID)
	if err != nil {
		return nil, err
	}

	return &site.DownloadRequest{
		URL: signedURL,
		Headers: http.Header{
			"User-Agent": {userAgent},
			"Referer":    {c.filePageURL(file.ID)},
		},
	}, nil
}

func (c *Cyberdrop) fetchInfo(ctx context.Context, slug string) (*fileInfo, error) {
	var info fileInfo
	if err := c.getJSON(ctx, c.apiURL()+"/api/file/info/"+url.PathEscape(slug), &info); err != nil {
		return nil, err
	}
	if info.Error != "" {
		return nil, fmt.Errorf("cyberdrop: %w: %s", site.ErrNotFound, info.Error)
	}
	if info.Name == "" {
		return nil, fmt.Errorf("cyberdrop: %w: missing file name for %s", site.ErrSiteChanged, slug)
	}
	return &info, nil
}

func (c *Cyberdrop) fetchSignedURL(ctx context.Context, slug string) (string, error) {
	var auth authResponse
	if err := c.getJSON(ctx, c.apiURL()+"/api/file/auth/"+url.PathEscape(slug), &auth); err != nil {
		return "", err
	}
	if auth.Error != "" {
		return "", fmt.Errorf("cyberdrop: %w: %s", site.ErrNotFound, auth.Error)
	}
	if auth.URL == "" {
		return "", fmt.Errorf("cyberdrop: %w: missing signed URL for %s", site.ErrSiteChanged, slug)
	}
	return auth.URL, nil
}

func (c *Cyberdrop) getJSON(ctx context.Context, apiURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", c.baseURL()+"/")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return fmt.Errorf("cyberdrop: %w: %s", site.ErrNotFound, apiURL)
	case http.StatusTooManyRequests:
		return fmt.Errorf("cyberdrop: %w", site.ErrRateLimited)
	case http.StatusForbidden:
		return fmt.Errorf("cyberdrop: %w: forbidden", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return fmt.Errorf("cyberdrop: unexpected status %d for %s", resp.StatusCode, apiURL)
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("cyberdrop: decode response: %w", err)
	}
	return nil
}

func parseFileSlug(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host != "cyberdrop.cr" && host != "www.cyberdrop.cr" {
		return "", false
	}

	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 2 || parts[0] != "f" || parts[1] == "" {
		return "", false
	}
	slug, err := url.PathUnescape(parts[1])
	if err != nil || slug == "" {
		return "", false
	}
	return slug, true
}

func (c *Cyberdrop) DefaultConcurrency() int             { return 3 }
func (c *Cyberdrop) DefaultResolveDelay() time.Duration  { return 0 }
func (c *Cyberdrop) DefaultDownloadDelay() time.Duration { return time.Second }

func (c *Cyberdrop) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Cyberdrop) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return defaultBaseURL
}

func (c *Cyberdrop) apiURL() string {
	if c.APIURL != "" {
		return strings.TrimRight(c.APIURL, "/")
	}
	return defaultAPIURL
}

func (c *Cyberdrop) filePageURL(slug string) string {
	return c.baseURL() + "/f/" + url.PathEscape(slug)
}

type fileInfo struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Size         int64  `json:"size"`
	Slug         string `json:"slug"`
	ThumbnailURL string `json:"thumbnail_url"`
	AuthURL      string `json:"auth_url"`
	Error        string `json:"error"`
}

type authResponse struct {
	URL   string `json:"url"`
	Error string `json:"error"`
}
