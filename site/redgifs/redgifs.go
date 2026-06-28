package redgifs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wasylq/MSD/site"
)

const (
	defaultBaseURL   = "https://www.redgifs.com"
	defaultAPIURL    = "https://api.redgifs.com"
	userAgent        = "Mozilla/5.0"
	pageSize         = 100
	maxPageCount     = 5000
	defaultListOrder = "new"
)

var errRefreshToken = errors.New("refresh token")

func init() { site.Register(&Redgifs{}) }

type Redgifs struct {
	HTTPClient *http.Client
	BaseURL    string
	APIBaseURL string

	mu    sync.Mutex
	token string
}

type listingRef struct {
	kind string
	id   string
}

func (r *Redgifs) Name() string { return "redgifs" }

func (r *Redgifs) Match(rawURL string) bool {
	return parseRedgifsURL(rawURL) != nil
}

func (r *Redgifs) Resolve(ctx context.Context, rawURL string, _ string) (*site.Album, error) {
	ref := parseRedgifsURL(rawURL)
	if ref == nil {
		return nil, fmt.Errorf("redgifs: %w: %s", site.ErrNotFound, rawURL)
	}

	switch ref.kind {
	case "user":
		return r.resolveListing(ctx, ref, "/v2/users/"+url.PathEscape(ref.id)+"/search")
	case "niche":
		return r.resolveListing(ctx, ref, "/v2/niches/"+url.PathEscape(ref.id)+"/gifs")
	default:
		return nil, fmt.Errorf("redgifs: %w: %s", site.ErrNotFound, rawURL)
	}
}

func (r *Redgifs) resolveListing(ctx context.Context, ref *listingRef, apiPath string) (*site.Album, error) {
	album := &site.Album{
		ID:   ref.kind + "-" + ref.id,
		Name: "redgifs-" + ref.kind + "-" + ref.id,
	}

	seen := make(map[string]struct{})
	for page := 1; page <= maxPageCount; page++ {
		result, err := r.fetchListingPage(ctx, apiPath, page)
		if err != nil {
			return nil, err
		}

		for _, gif := range result.Gifs {
			if gif.ID == "" {
				continue
			}
			if _, ok := seen[gif.ID]; ok {
				continue
			}
			seen[gif.ID] = struct{}{}

			downloadURL := gif.downloadURL()
			if downloadURL == "" {
				continue
			}

			album.PostLinks = append(album.PostLinks, r.baseURL()+"/watch/"+gif.ID)
			album.Files = append(album.Files, site.File{
				ID:   downloadURL,
				Name: gif.fileName(),
				Size: -1,
			})
		}

		if result.Pages <= 0 || page >= result.Pages {
			break
		}
	}

	if len(album.Files) == 0 {
		return nil, fmt.Errorf("redgifs: %w: no downloadable files for %s %s", site.ErrNotFound, ref.kind, ref.id)
	}
	return album, nil
}

func (r *Redgifs) fetchListingPage(ctx context.Context, apiPath string, page int) (*listingResponse, error) {
	apiURL, err := url.Parse(r.apiBaseURL() + apiPath)
	if err != nil {
		return nil, err
	}
	q := apiURL.Query()
	q.Set("count", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))
	q.Set("order", defaultListOrder)
	apiURL.RawQuery = q.Encode()

	var result listingResponse
	if err := r.getJSON(ctx, apiURL.String(), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *Redgifs) getJSON(ctx context.Context, apiURL string, dest any) error {
	if err := r.getJSONWithToken(ctx, apiURL, dest); err != nil {
		if !errors.Is(err, errRefreshToken) {
			return err
		}
		r.clearToken()
		return r.getJSONWithToken(ctx, apiURL, dest)
	}
	return nil
}

func (r *Redgifs) getJSONWithToken(ctx context.Context, apiURL string, dest any) error {
	token, err := r.authToken(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Referer", r.baseURL()+"/")

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return fmt.Errorf("redgifs: %w: unauthorized", errRefreshToken)
	case http.StatusNotFound:
		return fmt.Errorf("redgifs: %w: %s", site.ErrNotFound, apiURL)
	case http.StatusTooManyRequests:
		return fmt.Errorf("redgifs: %w", site.ErrRateLimited)
	case http.StatusForbidden:
		return fmt.Errorf("redgifs: %w: forbidden", site.ErrAuthRequired)
	default:
		if resp.StatusCode >= 400 {
			return fmt.Errorf("redgifs: unexpected status %d for %s", resp.StatusCode, apiURL)
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("redgifs: decode response: %w", err)
	}
	return nil
}

func (r *Redgifs) authToken(ctx context.Context) (string, error) {
	r.mu.Lock()
	token := r.token
	r.mu.Unlock()
	if token != "" {
		return token, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.apiBaseURL()+"/v2/auth/temporary", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return "", fmt.Errorf("redgifs: %w", site.ErrRateLimited)
	case http.StatusForbidden, http.StatusUnauthorized:
		return "", fmt.Errorf("redgifs: %w: token denied", site.ErrAuthRequired)
	default:
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("redgifs: token API returned %d", resp.StatusCode)
		}
	}

	var result authResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("redgifs: decode token response: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("redgifs: %w: missing temporary token", site.ErrSiteChanged)
	}

	r.mu.Lock()
	r.token = result.Token
	r.mu.Unlock()
	return result.Token, nil
}

func (r *Redgifs) clearToken() {
	r.mu.Lock()
	r.token = ""
	r.mu.Unlock()
}

func (r *Redgifs) DownloadRequest(_ context.Context, file site.File) (*site.DownloadRequest, error) {
	u, err := url.Parse(file.ID)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("redgifs: no download link for %s", file.ID)
	}
	return &site.DownloadRequest{
		URL: file.ID,
		Headers: http.Header{
			"User-Agent": {userAgent},
			"Referer":    {r.baseURL() + "/"},
		},
	}, nil
}

func (r *Redgifs) DefaultConcurrency() int             { return 2 }
func (r *Redgifs) DefaultResolveDelay() time.Duration  { return 0 }
func (r *Redgifs) DefaultDownloadDelay() time.Duration { return 1 * time.Second }

func (r *Redgifs) httpClient() *http.Client {
	if r.HTTPClient != nil {
		return r.HTTPClient
	}
	return http.DefaultClient
}

func (r *Redgifs) baseURL() string {
	if r.BaseURL != "" {
		return strings.TrimRight(r.BaseURL, "/")
	}
	return defaultBaseURL
}

func (r *Redgifs) apiBaseURL() string {
	if r.APIBaseURL != "" {
		return strings.TrimRight(r.APIBaseURL, "/")
	}
	return defaultAPIURL
}

func parseRedgifsURL(rawURL string) *listingRef {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	if host != "redgifs.com" && host != "www.redgifs.com" {
		return nil
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[1] == "" {
		return nil
	}
	id, err := url.PathUnescape(parts[1])
	if err != nil || id == "" {
		return nil
	}
	switch strings.ToLower(parts[0]) {
	case "users":
		return &listingRef{kind: "user", id: id}
	case "niches":
		return &listingRef{kind: "niche", id: id}
	default:
		return nil
	}
}

type authResponse struct {
	Token string `json:"token"`
}

type listingResponse struct {
	Gifs  []gifResponse `json:"gifs"`
	Page  int           `json:"page"`
	Pages int           `json:"pages"`
	Total int           `json:"total"`
}

type gifResponse struct {
	ID         string  `json:"id"`
	CreateDate int64   `json:"createDate"`
	UserName   string  `json:"userName"`
	URLs       gifURLs `json:"urls"`
}

func (g gifResponse) downloadURL() string {
	for _, candidate := range []string{g.URLs.HD, g.URLs.SD, g.URLs.Silent, g.URLs.Poster, g.URLs.Thumbnail} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func (g gifResponse) fileName() string {
	parts := []string{}
	if date := gifDate(g.CreateDate); date != "" {
		parts = append(parts, date)
	}
	if g.UserName != "" {
		parts = append(parts, g.UserName)
	}
	if g.ID != "" {
		parts = append(parts, g.ID)
	}
	name := strings.Join(parts, "_")
	if name == "" {
		name = "redgifs"
	}

	ext := ".mp4"
	if u, err := url.Parse(g.downloadURL()); err == nil {
		if pathExt := path.Ext(u.Path); pathExt != "" {
			ext = pathExt
		}
	}
	return name + ext
}

type gifURLs struct {
	HD        string `json:"hd"`
	SD        string `json:"sd"`
	Silent    string `json:"silent"`
	Poster    string `json:"poster"`
	Thumbnail string `json:"thumbnail"`
}

func gifDate(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format("20060102")
}
