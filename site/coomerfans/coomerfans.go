package coomerfans

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/Wasylq/MSD/site"
)

const (
	defaultBaseURL = "https://coomerfans.com"
	userAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
)

var mediaHostPattern = regexp.MustCompile(`(?i)(^|\.)coomerfans\.com$`)

func init() { site.Register(&CoomerFans{}) }

type CoomerFans struct {
	HTTPClient *http.Client
	BaseURL    string
}

type creatorURL struct {
	service   string
	creatorID string
	name      string
	origin    string
	path      string
}

type postRef struct {
	id      string
	url     string
	path    string
	title   string
	date    string
	service string
}

type mediaFile struct {
	url  string
	name string
}

func (c *CoomerFans) Name() string { return "coomerfans" }

func (c *CoomerFans) Match(rawURL string) bool {
	return parseCreatorURL(rawURL) != nil || parsePostURL(rawURL) != nil
}

func (c *CoomerFans) Resolve(ctx context.Context, rawURL string, _ string) (*site.Album, error) {
	if post := parsePostURL(rawURL); post != nil {
		return c.resolvePost(ctx, post, rawURL)
	}

	creator := parseCreatorURL(rawURL)
	if creator == nil {
		return nil, fmt.Errorf("coomerfans: %w: %s", site.ErrNotFound, rawURL)
	}

	album := &site.Album{
		ID:   creator.service + "-" + creator.creatorID,
		Name: "coomerfans-" + creator.service + "-" + creator.name,
	}

	seenPosts := make(map[string]struct{})
	seenPages := make(map[string]struct{})
	for pageURL := c.pageURL(creator.path, 1); pageURL != ""; {
		if _, ok := seenPages[pageURL]; ok {
			return nil, fmt.Errorf("coomerfans: %w: repeated pagination URL %s", site.ErrSiteChanged, pageURL)
		}
		seenPages[pageURL] = struct{}{}

		posts, nextPage, err := c.parseCreatorPage(ctx, pageURL, creator)
		if err != nil {
			return nil, err
		}
		if len(posts) == 0 && len(album.PostLinks) == 0 {
			return nil, fmt.Errorf("coomerfans: %w: no posts for %s", site.ErrNotFound, creator.creatorID)
		}

		for _, post := range posts {
			if _, ok := seenPosts[post.url]; ok {
				continue
			}
			seenPosts[post.url] = struct{}{}
			album.PostLinks = append(album.PostLinks, post.url)

			files, err := c.parsePostPage(ctx, c.baseURL()+post.path, post)
			if err != nil {
				return nil, err
			}
			album.Files = append(album.Files, files...)
		}

		pageURL = nextPage
	}

	if len(album.Files) == 0 {
		return nil, fmt.Errorf("coomerfans: %w: no downloadable files for %s", site.ErrNotFound, creator.creatorID)
	}

	return album, nil
}

func (c *CoomerFans) resolvePost(ctx context.Context, post *postRef, rawURL string) (*site.Album, error) {
	files, err := c.parsePostPage(ctx, c.baseURL()+post.path, *post)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("coomerfans: %w: no downloadable files for %s", site.ErrNotFound, post.id)
	}

	return &site.Album{
		ID:        post.id,
		Name:      "coomerfans-" + post.service + "-" + post.id,
		Files:     files,
		PostLinks: []string{rawURL},
	}, nil
}

func (c *CoomerFans) parseCreatorPage(ctx context.Context, pageURL string, creator *creatorURL) ([]postRef, string, error) {
	doc, err := c.getHTML(ctx, pageURL)
	if err != nil {
		return nil, "", err
	}

	var posts []postRef
	doc.Find(".posts-list .post").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Find("a.view-post[href]").First().Attr("href")
		if href == "" {
			href, _ = s.Find("h3 a[href]").First().Attr("href")
		}
		post := parsePostPath(href)
		if post == nil {
			return
		}
		post.url = creator.origin + post.path
		post.title = strings.TrimSpace(s.Find("h3 a").First().Text())
		post.date = postDateText(s)
		posts = append(posts, *post)
	})

	nextPage := ""
	if href, ok := doc.Find(".pagination a.next[href]").First().Attr("href"); ok {
		if u, err := url.Parse(href); err == nil {
			nextPage = c.baseURL() + u.RequestURI()
		}
	}

	return posts, nextPage, nil
}

func (c *CoomerFans) parsePostPage(ctx context.Context, pageURL string, ref postRef) ([]site.File, error) {
	doc, err := c.getHTML(ctx, pageURL)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(doc.Find(".post-wrap h1").First().Text())
	if title == "" {
		title = ref.title
	}
	date := postDateText(doc.Find(".post-wrap").First())
	if date == "" {
		date = ref.date
	}

	var files []site.File
	seen := make(map[string]struct{})
	for _, media := range extractMedia(doc) {
		if _, ok := seen[media.url]; ok {
			continue
		}
		seen[media.url] = struct{}{}

		index := len(files) + 1
		name := postFileName(ref.id, date, title, index, media.name)
		files = append(files, site.File{
			ID:   media.url,
			Name: name,
			Size: -1,
		})
	}

	return files, nil
}

func (c *CoomerFans) getHTML(ctx context.Context, pageURL string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Referer", c.baseURL()+"/")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("coomerfans: %w: %s", site.ErrNotFound, pageURL)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("coomerfans: %w", site.ErrRateLimited)
	case http.StatusForbidden:
		return nil, fmt.Errorf("coomerfans: %w: forbidden", site.ErrRateLimited)
	default:
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("coomerfans: unexpected status %d for %s", resp.StatusCode, pageURL)
		}
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("coomerfans: parse HTML: %w", err)
	}
	return doc, nil
}

func extractMedia(doc *goquery.Document) []mediaFile {
	var files []mediaFile
	doc.Find(".post-body img[src], .post-body video[src], .post-body video source[src], .post-body a[href]").Each(func(_ int, s *goquery.Selection) {
		raw := ""
		if src, ok := s.Attr("src"); ok {
			raw = src
		} else if href, ok := s.Attr("href"); ok {
			raw = href
		}

		link, ok := mediaURL(raw)
		if !ok {
			return
		}
		u, err := url.Parse(link)
		if err != nil {
			return
		}
		files = append(files, mediaFile{
			url:  link,
			name: path.Base(u.Path),
		})
	})
	return files
}

func mediaURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	if !mediaHostPattern.MatchString(strings.ToLower(u.Hostname())) {
		return "", false
	}
	if !strings.Contains(u.Path, "/storage/") {
		return "", false
	}
	return u.String(), true
}

func postDateText(s *goquery.Selection) string {
	text := strings.TrimSpace(s.Find(".post-date").First().Text())
	text = strings.TrimPrefix(text, "Added ")
	if len(text) >= len("2006-01-02") {
		return text[:len("2006-01-02")]
	}
	return ""
}

func postFileName(postID, date, title string, index int, name string) string {
	parts := []string{}
	if date != "" {
		parts = append(parts, date)
	}
	if title != "" {
		parts = append(parts, title)
	}
	if postID != "" {
		parts = append(parts, postID)
	}
	parts = append(parts, fmt.Sprintf("%02d", index))
	if name != "" && name != "." && name != "/" {
		parts = append(parts, name)
	}
	return strings.Join(parts, " - ")
}

func parseCreatorURL(rawURL string) *creatorURL {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	if strings.ToLower(u.Hostname()) != "coomerfans.com" {
		return nil
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "u" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return nil
	}
	name, err := url.PathUnescape(parts[3])
	if err != nil || name == "" {
		name = parts[3]
	}

	return &creatorURL{
		service:   parts[1],
		creatorID: parts[2],
		name:      name,
		origin:    u.Scheme + "://" + u.Host,
		path:      "/" + strings.Join(parts[:4], "/"),
	}
}

func parsePostURL(rawURL string) *postRef {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	if strings.ToLower(u.Hostname()) != "coomerfans.com" {
		return nil
	}
	return parsePostPath(u.Path)
}

func parsePostPath(rawPath string) *postRef {
	parts := strings.Split(strings.Trim(rawPath, "/"), "/")
	if len(parts) < 4 || parts[0] != "p" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return nil
	}
	return &postRef{
		id:      parts[1],
		path:    "/" + strings.Join(parts[:4], "/"),
		service: parts[3],
	}
}

func (c *CoomerFans) DownloadRequest(_ context.Context, file site.File) (*site.DownloadRequest, error) {
	if _, ok := mediaURL(file.ID); !ok {
		return nil, fmt.Errorf("coomerfans: no download link for %s", file.ID)
	}
	return &site.DownloadRequest{
		URL: file.ID,
		Headers: http.Header{
			"User-Agent": {userAgent},
			"Referer":    {c.baseURL() + "/"},
		},
	}, nil
}

func (c *CoomerFans) DefaultConcurrency() int             { return 3 }
func (c *CoomerFans) DefaultResolveDelay() time.Duration  { return time.Second }
func (c *CoomerFans) DefaultDownloadDelay() time.Duration { return 500 * time.Millisecond }

func (c *CoomerFans) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *CoomerFans) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return defaultBaseURL
}

func (c *CoomerFans) pageURL(creatorPath string, page int) string {
	if page <= 1 {
		return c.baseURL() + creatorPath
	}
	return fmt.Sprintf("%s%s?page=%d", c.baseURL(), creatorPath, page)
}
