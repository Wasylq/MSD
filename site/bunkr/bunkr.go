package bunkr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/Wasylq/MSD/site"
)

const (
	defaultBaseURL   = "https://bunkr.cr"
	defaultBridgeURL = "https://dl.bunkr.cr/api/_001_v2"
	defaultSignURL   = "https://glb-apisign.cdn.cr/sign"
	userAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
)

var (
	idPattern           = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	albumFilesPattern   = regexp.MustCompile(`(?s)window\.albumFiles\s*=\s*(\[.*?\]);`)
	albumFileObjPattern = regexp.MustCompile(`(?s)\{(.*?)\}`)
	fileIDPatterns      = []*regexp.Regexp{
		regexp.MustCompile(`data-file-id\s*=\s*["']?(\d+)["']?`),
		regexp.MustCompile(`data-id\s*=\s*["']?(\d+)["']?`),
		regexp.MustCompile(`/file/(\d+)`),
	}
)

func init() { site.Register(&Bunkr{}) }

type Bunkr struct {
	HTTPClient *http.Client
	BaseURL    string
	BridgeURL  string
	SignURL    string

	mu    sync.Mutex
	slugs map[string]string // numeric file ID -> album slug
}

func (b *Bunkr) Name() string { return "bunkr" }

func (b *Bunkr) Match(rawURL string) bool {
	_, _, err := parseBunkrURL(rawURL)
	return err == nil
}

func (b *Bunkr) Resolve(ctx context.Context, rawURL string, _ string) (*site.Album, error) {
	kind, id, err := parseBunkrURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("bunkr: %w: %s", site.ErrNotFound, rawURL)
	}

	if kind == "a" {
		return b.resolveAlbum(ctx, id)
	}
	return b.resolveFile(ctx, kind, id)
}

func (b *Bunkr) resolveAlbum(ctx context.Context, id string) (*site.Album, error) {
	pageURL := b.albumURL(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", b.baseURL()+"/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := b.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("bunkr: %w: album %s", site.ErrNotFound, id)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("bunkr: %w", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("bunkr: unexpected album status %d", resp.StatusCode)
		}
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bunkr: parse HTML: %w", err)
	}

	files := extractAlbumFiles(doc)
	if len(files) == 0 {
		return nil, fmt.Errorf("bunkr: %w: no files in album %s", site.ErrNotFound, id)
	}

	b.mu.Lock()
	b.slugs = make(map[string]string, len(files))
	for _, f := range files {
		if f.slug != "" {
			b.slugs[f.file.ID] = f.slug
		}
	}
	b.mu.Unlock()

	album := &site.Album{
		ID:    id,
		Name:  extractAlbumTitle(doc),
		Files: make([]site.File, 0, len(files)),
	}
	for _, f := range files {
		album.Files = append(album.Files, f.file)
	}
	return album, nil
}

func (b *Bunkr) resolveFile(ctx context.Context, kind, slug string) (*site.Album, error) {
	pageURL := b.baseURL() + "/" + kind + "/" + slug
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", b.baseURL()+"/")

	resp, err := b.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("bunkr: %w: file %s", site.ErrNotFound, slug)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("bunkr: %w", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("bunkr: unexpected file status %d", resp.StatusCode)
		}
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bunkr: parse file HTML: %w", err)
	}

	html, err := doc.Html()
	if err != nil {
		return nil, fmt.Errorf("bunkr: serialize file HTML: %w", err)
	}
	fileID := extractFileID(html)
	if fileID == "" {
		return nil, fmt.Errorf("bunkr: %w: missing file id on %s", site.ErrSiteChanged, pageURL)
	}

	name := extractFileTitle(doc)
	if name == "" {
		name = slug
	}

	b.mu.Lock()
	if b.slugs == nil {
		b.slugs = make(map[string]string)
	}
	b.slugs[fileID] = slug
	b.mu.Unlock()

	return &site.Album{
		ID: slug,
		Files: []site.File{{
			ID:   fileID,
			Name: name,
			Size: -1,
		}},
	}, nil
}

type albumFile struct {
	file site.File
	slug string
}

func extractAlbumFiles(doc *goquery.Document) []albumFile {
	html, err := doc.Html()
	if err != nil {
		return nil
	}

	m := albumFilesPattern.FindStringSubmatch(html)
	if m == nil {
		return nil
	}

	var files []albumFile
	for _, obj := range albumFileObjPattern.FindAllStringSubmatch(m[1], -1) {
		if len(obj) < 2 {
			continue
		}
		body := obj[1]
		id := extractNumberField(body, "id")
		slug := extractStringField(body, "slug")
		if id == "" || slug == "" {
			continue
		}

		name := extractStringField(body, "original")
		if name == "" {
			name = extractStringField(body, "name")
		}
		if name == "" {
			name = slug
		}

		var size int64 = -1
		if sizeStr := extractNumberField(body, "size"); sizeStr != "" {
			if n, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
				size = n
			}
		}

		files = append(files, albumFile{
			file: site.File{
				ID:   id,
				Name: name,
				Size: size,
			},
			slug: slug,
		})
	}

	return files
}

func extractAlbumTitle(doc *goquery.Document) string {
	if title := strings.TrimSpace(doc.Find(".sm\\:text-lg h1").First().Text()); title != "" {
		return title
	}
	if title := strings.TrimSpace(doc.Find("h1").First().Text()); title != "" {
		return title
	}

	title := strings.TrimSpace(doc.Find("title").First().Text())
	title = strings.TrimSuffix(title, " | Bunkr")
	return strings.TrimSpace(title)
}

func extractFileTitle(doc *goquery.Document) string {
	if title := strings.TrimSpace(doc.Find("h1").First().Text()); title != "" {
		return title
	}

	title := strings.TrimSpace(doc.Find("title").First().Text())
	title = strings.TrimPrefix(title, "Download ")
	title = strings.TrimSuffix(title, " | Bunkr")
	return strings.TrimSpace(title)
}

func extractFileID(html string) string {
	for _, pattern := range fileIDPatterns {
		if m := pattern.FindStringSubmatch(html); m != nil {
			return m[1]
		}
	}
	return ""
}

func extractStringField(body, key string) string {
	pattern := regexp.MustCompile(`(?s)\b` + regexp.QuoteMeta(key) + `\s*:\s*("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*')`)
	m := pattern.FindStringSubmatch(body)
	if m == nil {
		return ""
	}

	raw := m[1]
	if strings.HasPrefix(raw, "'") {
		raw = `"` + strings.ReplaceAll(strings.Trim(raw, "'"), `"`, `\"`) + `"`
	}
	v, err := strconv.Unquote(raw)
	if err != nil {
		return strings.Trim(raw, `"'`)
	}
	return v
}

func extractNumberField(body, key string) string {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `\s*:\s*(\d+)`)
	m := pattern.FindStringSubmatch(body)
	if m == nil {
		return ""
	}
	return m[1]
}

func (b *Bunkr) DownloadRequest(ctx context.Context, file site.File) (*site.DownloadRequest, error) {
	bridge, err := b.bridge(ctx, file.ID)
	if err != nil {
		return nil, err
	}

	rawURL, err := bridge.mediaURL()
	if err != nil {
		return nil, fmt.Errorf("bunkr: bad bridge response for %s: %w", file.ID, err)
	}

	name := bridge.Original
	if name == "" {
		name = file.Name
	}
	if name != "" {
		q := rawURL.Query()
		q.Set("n", name)
		rawURL.RawQuery = q.Encode()
	}

	signedURL, err := b.sign(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	referer := b.baseURL() + "/"
	b.mu.Lock()
	if slug := b.slugs[file.ID]; slug != "" {
		referer = b.baseURL() + "/f/" + slug
	}
	b.mu.Unlock()

	return &site.DownloadRequest{
		URL: signedURL,
		Headers: http.Header{
			"User-Agent": {userAgent},
			"Referer":    {referer},
		},
	}, nil
}

func (b *Bunkr) bridge(ctx context.Context, id string) (*bridgeResponse, error) {
	body, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.bridgeURL(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", "https://dl.bunkr.cr")
	req.Header.Set("Referer", "https://dl.bunkr.cr/file/"+id)

	resp, err := b.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("bunkr: %w: file %s", site.ErrNotFound, id)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("bunkr: %w", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("bunkr: bridge API returned %d for %s", resp.StatusCode, id)
		}
	}

	var result bridgeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("bunkr: decode bridge response: %w", err)
	}
	if result.MediaFiles == "" || result.Path == "" {
		return nil, fmt.Errorf("bunkr: bridge API failed for %s: missing media URL", id)
	}
	return &result, nil
}

type bridgeResponse struct {
	MediaFiles string `json:"mediafiles"`
	Path       string `json:"path"`
	Original   string `json:"original"`
}

func (r *bridgeResponse) mediaURL() (*url.URL, error) {
	base := strings.TrimRight(r.MediaFiles, "/")
	path := "/" + strings.TrimLeft(r.Path, "/")
	return url.Parse(base + path)
}

func (b *Bunkr) sign(ctx context.Context, mediaURL *url.URL) (string, error) {
	signURL, err := url.Parse(b.signURL())
	if err != nil {
		return "", err
	}
	q := signURL.Query()
	q.Set("path", mediaURL.EscapedPath())
	signURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", b.baseURL()+"/")

	resp, err := b.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return "", fmt.Errorf("bunkr: %w", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("bunkr: signer returned %d for %s", resp.StatusCode, mediaURL.Path)
		}
	}

	var result signResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("bunkr: decode sign response: %w", err)
	}
	if result.Token == "" || result.Ex == 0 {
		return "", fmt.Errorf("bunkr: signer failed for %s", mediaURL.Path)
	}

	q = mediaURL.Query()
	q.Set("token", result.Token)
	q.Set("ex", strconv.FormatInt(result.Ex, 10))
	mediaURL.RawQuery = q.Encode()
	return mediaURL.String(), nil
}

type signResponse struct {
	Token string `json:"token"`
	Ex    int64  `json:"ex"`
}

func (b *Bunkr) DefaultConcurrency() int             { return 2 }
func (b *Bunkr) DefaultResolveDelay() time.Duration  { return 5 * time.Second }
func (b *Bunkr) DefaultDownloadDelay() time.Duration { return 2 * time.Second }

func (b *Bunkr) httpClient() *http.Client {
	if b.HTTPClient != nil {
		return b.HTTPClient
	}
	return http.DefaultClient
}

func (b *Bunkr) baseURL() string {
	if b.BaseURL != "" {
		return strings.TrimRight(b.BaseURL, "/")
	}
	return defaultBaseURL
}

func (b *Bunkr) bridgeURL() string {
	if b.BridgeURL != "" {
		return b.BridgeURL
	}
	return defaultBridgeURL
}

func (b *Bunkr) signURL() string {
	if b.SignURL != "" {
		return b.SignURL
	}
	return defaultSignURL
}

func (b *Bunkr) albumURL(id string) string {
	u, _ := url.Parse(b.baseURL() + "/a/" + id)
	q := u.Query()
	q.Set("advanced", "1")
	u.RawQuery = q.Encode()
	return u.String()
}

func parseBunkrURL(rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("invalid URL")
	}

	if !isBunkrHost(strings.ToLower(u.Hostname())) {
		return "", "", fmt.Errorf("unsupported host")
	}

	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid path")
	}
	kind, id := parts[0], parts[1]
	if kind != "a" && kind != "f" && kind != "i" && kind != "v" {
		return "", "", fmt.Errorf("unsupported path")
	}
	if !idPattern.MatchString(id) {
		return "", "", fmt.Errorf("invalid ID")
	}
	return kind, id, nil
}

func isBunkrHost(host string) bool {
	return strings.HasPrefix(host, "bunkr.") ||
		strings.HasPrefix(host, "bunkrr.") ||
		host == "bunkr.cr" ||
		host == "www.bunkr.cr" ||
		host == "bunkr-albums.io" ||
		host == "www.bunkr-albums.io" ||
		host == "balbums.st" ||
		host == "www.balbums.st"
}
