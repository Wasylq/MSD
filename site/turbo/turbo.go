package turbo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/Wasylq/MSD/site"
)

const (
	defaultBaseURL = "https://turbo.cr"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
)

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func init() { site.Register(&Turbo{}) }

type Turbo struct {
	HTTPClient *http.Client
	BaseURL    string
}

func (t *Turbo) Name() string { return "turbo" }

func (t *Turbo) Match(rawURL string) bool {
	_, _, err := parseTurboURL(rawURL)
	return err == nil
}

func (t *Turbo) Resolve(ctx context.Context, rawURL string, _ string) (*site.Album, error) {
	kind, id, err := parseTurboURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("turbo: %w: %s", site.ErrNotFound, rawURL)
	}

	if kind == "a" {
		return t.resolveAlbum(ctx, id)
	}
	return t.resolveFile(ctx, id)
}

func (t *Turbo) resolveAlbum(ctx context.Context, id string) (*site.Album, error) {
	pageURL := t.baseURL() + "/a/" + id
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", t.baseURL()+"/")

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("turbo: %w: album %s", site.ErrNotFound, id)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("turbo: %w", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("turbo: unexpected album status %d", resp.StatusCode)
		}
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("turbo: parse HTML: %w", err)
	}

	files := extractAlbumFiles(doc)
	if len(files) == 0 {
		return nil, fmt.Errorf("turbo: %w: no files in album %s", site.ErrNotFound, id)
	}

	return &site.Album{
		ID:    id,
		Name:  extractAlbumTitle(doc),
		Files: files,
	}, nil
}

func (t *Turbo) resolveFile(ctx context.Context, id string) (*site.Album, error) {
	signed, err := t.sign(ctx, id)
	if err != nil {
		return nil, err
	}

	name := signed.OriginalFilename
	if name == "" {
		name = signed.Filename
	}
	if name == "" {
		name = id
	}

	return &site.Album{
		ID: id,
		Files: []site.File{{
			ID:   id,
			Name: name,
			Size: -1,
		}},
	}, nil
}

func extractAlbumFiles(doc *goquery.Document) []site.File {
	var files []site.File

	doc.Find(".file-row[data-id][data-name]").Each(func(_ int, s *goquery.Selection) {
		id, _ := s.Attr("data-id")
		name, _ := s.Attr("data-name")
		if id == "" || name == "" {
			return
		}

		var size int64 = -1
		if sizeStr, ok := s.Attr("data-size"); ok {
			if n, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
				size = n
			}
		}

		files = append(files, site.File{
			ID:   id,
			Name: name,
			Size: size,
		})
	})

	return files
}

func extractAlbumTitle(doc *goquery.Document) string {
	if title := strings.TrimSpace(doc.Find("h1").First().Text()); title != "" {
		return title
	}

	title := strings.TrimSpace(doc.Find("title").First().Text())
	title = strings.TrimSuffix(title, " - turbo.cr")
	return strings.TrimSpace(title)
}

func (t *Turbo) DownloadRequest(ctx context.Context, file site.File) (*site.DownloadRequest, error) {
	signed, err := t.sign(ctx, file.ID)
	if err != nil {
		return nil, err
	}

	downloadURL, err := withDownloadParam(signed.URL)
	if err != nil {
		return nil, fmt.Errorf("turbo: bad signed URL for %s: %w", file.ID, err)
	}

	return &site.DownloadRequest{
		URL: downloadURL,
		Headers: http.Header{
			"User-Agent": {userAgent},
			"Referer":    {t.baseURL() + "/d/" + file.ID},
		},
	}, nil
}

func (t *Turbo) sign(ctx context.Context, id string) (*signResponse, error) {
	signURL, err := url.Parse(t.baseURL() + "/api/sign")
	if err != nil {
		return nil, err
	}
	q := signURL.Query()
	q.Set("v", id)
	signURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", t.baseURL()+"/d/"+id)

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("turbo: %w: file %s", site.ErrNotFound, id)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("turbo: %w", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("turbo: sign API returned %d for %s", resp.StatusCode, id)
		}
	}

	var result signResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("turbo: decode sign response: %w", err)
	}

	if !result.Success || result.URL == "" {
		msg := result.Error
		if msg == "" {
			msg = result.Message
		}
		if looksLikeChallenge(msg) {
			return nil, fmt.Errorf("turbo: %w: %s", site.ErrAuthRequired, msg)
		}
		if msg == "" {
			msg = "missing signed URL"
		}
		return nil, fmt.Errorf("turbo: sign API failed for %s: %s", id, msg)
	}

	return &result, nil
}

type signResponse struct {
	Success          bool   `json:"success"`
	URL              string `json:"url"`
	Filename         string `json:"filename"`
	OriginalFilename string `json:"original_filename"`
	Error            string `json:"error"`
	Message          string `json:"message"`
}

func looksLikeChallenge(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "captcha") ||
		strings.Contains(msg, "altcha") ||
		strings.Contains(msg, "challenge") ||
		strings.Contains(msg, "auth")
}

func withDownloadParam(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("dl", "1")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (t *Turbo) DefaultConcurrency() int             { return 2 }
func (t *Turbo) DefaultResolveDelay() time.Duration  { return 5 * time.Second }
func (t *Turbo) DefaultDownloadDelay() time.Duration { return 2 * time.Second }

func (t *Turbo) httpClient() *http.Client {
	if t.HTTPClient != nil {
		return t.HTTPClient
	}
	return http.DefaultClient
}

func (t *Turbo) baseURL() string {
	if t.BaseURL != "" {
		return strings.TrimRight(t.BaseURL, "/")
	}
	return defaultBaseURL
}

func parseTurboURL(rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("invalid URL")
	}

	host := strings.ToLower(u.Hostname())
	if host != "turbo.cr" && host != "www.turbo.cr" {
		return "", "", fmt.Errorf("unsupported host")
	}

	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid path")
	}
	kind, id := parts[0], parts[1]
	if kind != "a" && kind != "d" && kind != "v" {
		return "", "", fmt.Errorf("unsupported path")
	}
	if !idPattern.MatchString(id) {
		return "", "", fmt.Errorf("invalid ID")
	}
	return kind, id, nil
}
