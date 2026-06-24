package pixeldrain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/Wasylq/MSD/site"
)

const defaultBaseURL = "https://pixeldrain.com"

var (
	listPattern = regexp.MustCompile(`pixeldrain\.com/l/([a-zA-Z0-9]+)`)
	filePattern = regexp.MustCompile(`pixeldrain\.com/u/([a-zA-Z0-9]+)`)
)

func init() { site.Register(&Pixeldrain{}) }

type Pixeldrain struct {
	HTTPClient *http.Client
	BaseURL    string // for testing; defaults to https://pixeldrain.com
}

func (p *Pixeldrain) Name() string { return "pixeldrain" }

func (p *Pixeldrain) Match(url string) bool {
	return listPattern.MatchString(url) || filePattern.MatchString(url)
}

func (p *Pixeldrain) Resolve(ctx context.Context, url string, _ string) (*site.Album, error) {
	if m := listPattern.FindStringSubmatch(url); m != nil {
		return p.resolveList(ctx, m[1])
	}
	if m := filePattern.FindStringSubmatch(url); m != nil {
		return p.resolveFile(ctx, m[1])
	}
	return nil, fmt.Errorf("pixeldrain: %w: %s", site.ErrNotFound, url)
}

func (p *Pixeldrain) resolveList(ctx context.Context, id string) (*site.Album, error) {
	apiURL := p.baseURL() + "/api/list/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkStatus(resp.StatusCode, "list", id); err != nil {
		return nil, err
	}

	var apiResp listResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("pixeldrain: decode response: %w", err)
	}

	album := &site.Album{
		ID:   apiResp.ID,
		Name: apiResp.Title,
	}
	for _, f := range apiResp.Files {
		album.Files = append(album.Files, site.File{
			ID:   f.ID,
			Name: f.Name,
			Size: f.Size,
		})
	}
	return album, nil
}

func (p *Pixeldrain) resolveFile(ctx context.Context, id string) (*site.Album, error) {
	apiURL := p.baseURL() + "/api/file/" + id + "/info"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkStatus(resp.StatusCode, "file", id); err != nil {
		return nil, err
	}

	var f fileResponse
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return nil, fmt.Errorf("pixeldrain: decode response: %w", err)
	}

	return &site.Album{
		ID: f.ID,
		Files: []site.File{
			{ID: f.ID, Name: f.Name, Size: f.Size},
		},
	}, nil
}

func checkStatus(code int, kind, id string) error {
	switch code {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("pixeldrain: %w: %s %s", site.ErrNotFound, kind, id)
	case http.StatusTooManyRequests:
		return fmt.Errorf("pixeldrain: %w", site.ErrRateLimited)
	default:
		if code >= 400 {
			return fmt.Errorf("pixeldrain: unexpected status %d for %s %s", code, kind, id)
		}
		return nil
	}
}

func (p *Pixeldrain) DownloadRequest(_ context.Context, file site.File) (*site.DownloadRequest, error) {
	return &site.DownloadRequest{
		URL: p.baseURL() + "/api/file/" + file.ID + "?download",
	}, nil
}

func (p *Pixeldrain) DefaultConcurrency() int             { return 5 }
func (p *Pixeldrain) DefaultResolveDelay() time.Duration  { return 0 }
func (p *Pixeldrain) DefaultDownloadDelay() time.Duration { return 0 }

func (p *Pixeldrain) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return http.DefaultClient
}

func (p *Pixeldrain) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return defaultBaseURL
}

type listResponse struct {
	ID    string         `json:"id"`
	Title string         `json:"title"`
	Files []fileResponse `json:"files"`
}

type fileResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}
