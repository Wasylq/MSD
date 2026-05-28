package filester

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/Wasylq/MSD/site"
)

const (
	defaultBaseURL = "https://filester.me"
	defaultCDNURL  = "https://cache6.filester.me"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

var albumPattern = regexp.MustCompile(`filester\.me/f/([a-zA-Z0-9_-]+)`)

func init() { site.Register(&Filester{}) }

type Filester struct {
	HTTPClient *http.Client
	BaseURL    string
	CDNURL     string
}

func (f *Filester) Name() string { return "filester" }

func (f *Filester) Match(url string) bool {
	return albumPattern.MatchString(url)
}

func (f *Filester) Resolve(ctx context.Context, url string, _ string) (*site.Album, error) {
	m := albumPattern.FindStringSubmatch(url)
	if m == nil {
		return nil, fmt.Errorf("filester: %w: %s", site.ErrNotFound, url)
	}
	slug := m[1]

	album := &site.Album{ID: slug}

	for page := 1; ; page++ {
		pageURL := fmt.Sprintf("%s/f/%s?page=%d", f.baseURL(), slug, page)
		title, files, hasNext, err := f.parsePage(ctx, pageURL)
		if err != nil {
			return nil, err
		}

		if page == 1 {
			album.Name = title
			if len(files) == 0 {
				return nil, fmt.Errorf("filester: %w: album %s", site.ErrNotFound, slug)
			}
		}

		album.Files = append(album.Files, files...)
		if !hasNext {
			break
		}
	}

	return album, nil
}

func (f *Filester) parsePage(ctx context.Context, pageURL string) (string, []site.File, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", nil, false, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", f.baseURL()+"/")

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return "", nil, false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", nil, false, fmt.Errorf("filester: %w: %s", site.ErrNotFound, pageURL)
	case http.StatusTooManyRequests:
		return "", nil, false, fmt.Errorf("filester: %w", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return "", nil, false, fmt.Errorf("filester: unexpected status %d", resp.StatusCode)
		}
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", nil, false, fmt.Errorf("filester: parse HTML: %w", err)
	}

	title := strings.TrimSpace(doc.Find(".folder-title").First().Text())
	files, hasNext := extractFiles(doc)
	return title, files, hasNext, nil
}

var slugPattern = regexp.MustCompile(`/d/([a-zA-Z0-9_-]+)`)

func extractFiles(doc *goquery.Document) ([]site.File, bool) {
	var files []site.File

	doc.Find(".file-item[data-name]").Each(func(_ int, s *goquery.Selection) {
		name, _ := s.Attr("data-name")
		if name == "" {
			return
		}

		onclick, _ := s.Attr("onclick")
		slug := extractSlug(onclick)
		if slug == "" {
			return
		}

		var size int64 = -1
		if sizeStr, ok := s.Attr("data-size"); ok {
			if n, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
				size = n
			}
		}

		files = append(files, site.File{
			ID:   slug,
			Name: name,
			Size: size,
		})
	})

	hasNext := false
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		if strings.Contains(href, "?page=") {
			hasNext = true
		}
	})

	return files, hasNext
}

func extractSlug(s string) string {
	m := slugPattern.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

func (f *Filester) DownloadRequest(ctx context.Context, file site.File) (*site.DownloadRequest, error) {
	body, err := json.Marshal(map[string]string{"file_slug": file.ID})
	if err != nil {
		return nil, err
	}

	apiURL := f.baseURL() + "/api/public/view"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", f.baseURL()+"/")

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("filester: %w", site.ErrRateLimited)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("filester: view API returned %d for %s", resp.StatusCode, file.ID)
	}

	var viewResp struct {
		Success bool   `json:"success"`
		ViewURL string `json:"view_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&viewResp); err != nil {
		return nil, fmt.Errorf("filester: decode view response: %w", err)
	}
	if !viewResp.Success || viewResp.ViewURL == "" {
		return nil, fmt.Errorf("filester: failed to get download URL for %s", file.ID)
	}

	return &site.DownloadRequest{
		URL: f.cdnURL() + viewResp.ViewURL,
		Headers: http.Header{
			"User-Agent": {userAgent},
			"Referer":    {f.baseURL() + "/"},
		},
	}, nil
}

func (f *Filester) DefaultConcurrency() int            { return 3 }
func (f *Filester) DefaultResolveDelay() time.Duration  { return 5 * time.Second }
func (f *Filester) DefaultDownloadDelay() time.Duration { return 0 }

func (f *Filester) httpClient() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return http.DefaultClient
}

func (f *Filester) baseURL() string {
	if f.BaseURL != "" {
		return f.BaseURL
	}
	return defaultBaseURL
}

func (f *Filester) cdnURL() string {
	if f.CDNURL != "" {
		return f.CDNURL
	}
	return defaultCDNURL
}
