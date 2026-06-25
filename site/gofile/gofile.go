package gofile

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wasylq/MSD/site"
)

const (
	defaultAPIURL = "https://api.gofile.io"
	userAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
	tokenSeed     = "4fd6sg89d7s6"
	tokenWindow   = 14400 // 4 hours in seconds
)

var contentPattern = regexp.MustCompile(`^/d/([a-zA-Z0-9]+)`)

func init() { site.Register(&Gofile{}) }

type Gofile struct {
	HTTPClient *http.Client
	APIURL     string

	mu           sync.Mutex
	accountToken string
	links        map[string]string // file ID -> download link
}

func (g *Gofile) Name() string { return "gofile" }

func (g *Gofile) Match(rawURL string) bool {
	_, err := parseContentID(rawURL)
	return err == nil
}

func (g *Gofile) SetAccountToken(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.accountToken = token
}

func (g *Gofile) Resolve(ctx context.Context, rawURL string, password string) (*site.Album, error) {
	contentID, err := parseContentID(rawURL)
	if err != nil {
		return nil, fmt.Errorf("gofile: %w: %s", site.ErrNotFound, rawURL)
	}

	if err := g.ensureAccount(ctx); err != nil {
		return nil, fmt.Errorf("gofile: create account: %w", err)
	}

	g.mu.Lock()
	g.links = make(map[string]string)
	g.mu.Unlock()

	data, err := g.fetchContents(ctx, contentID, password)
	if err != nil {
		return nil, err
	}

	album := &site.Album{
		ID:   contentID,
		Name: data.Name,
	}

	g.collectFiles(data, album)

	if len(album.Files) == 0 {
		return nil, fmt.Errorf("gofile: %w: no files in %s", site.ErrNotFound, contentID)
	}

	return album, nil
}

func (g *Gofile) collectFiles(data *contentsData, album *site.Album) {
	if data.Type == "file" {
		g.addFile(data.ID, data.Name, data.Size, data.Link, album)
		return
	}

	for _, child := range data.Children {
		switch child.Type {
		case "file":
			g.addFile(child.ID, child.Name, child.Size, child.Link, album)
		case "folder":
			g.collectFiles(&contentsData{
				ID:       child.ID,
				Name:     child.Name,
				Type:     child.Type,
				Children: child.Children,
			}, album)
		}
	}
}

func (g *Gofile) addFile(id, name string, size int64, link string, album *site.Album) {
	if id == "" || link == "" {
		return
	}
	g.mu.Lock()
	g.links[id] = link
	g.mu.Unlock()
	album.Files = append(album.Files, site.File{
		ID:   id,
		Name: name,
		Size: size,
	})
}

func (g *Gofile) DownloadRequest(_ context.Context, file site.File) (*site.DownloadRequest, error) {
	g.mu.Lock()
	link := g.links[file.ID]
	token := g.accountToken
	g.mu.Unlock()

	if link == "" {
		return nil, fmt.Errorf("gofile: no download link for %s", file.ID)
	}

	return &site.DownloadRequest{
		URL: link,
		Headers: http.Header{
			"User-Agent": {userAgent},
			"Referer":    {"https://gofile.io/"},
		},
		Cookies: []*http.Cookie{
			{Name: "accountToken", Value: token},
		},
	}, nil
}

func (g *Gofile) DefaultConcurrency() int             { return 2 }
func (g *Gofile) DefaultResolveDelay() time.Duration  { return 10 * time.Second }
func (g *Gofile) DefaultDownloadDelay() time.Duration { return 10 * time.Second }

// --- Account management ---

func (g *Gofile) ensureAccount(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.accountToken != "" {
		return nil
	}

	if token := envAccountToken(); token != "" {
		g.accountToken = token
		return nil
	}

	if token, err := loadCachedToken(); err == nil && token != "" {
		g.accountToken = token
		return nil
	}

	token, err := g.createAccount(ctx)
	if err != nil {
		return err
	}
	g.accountToken = token
	_ = saveCachedToken(token)
	return nil
}

func (g *Gofile) createAccount(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.apiURL()+"/accounts", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("%w: account creation", site.ErrRateLimited)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("account creation failed: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Status != "ok" || result.Data.Token == "" {
		return "", fmt.Errorf("account creation failed: %s", result.Status)
	}
	return result.Data.Token, nil
}

// --- Contents API ---

func (g *Gofile) fetchContents(ctx context.Context, contentID, password string) (*contentsData, error) {
	g.mu.Lock()
	token := g.accountToken
	g.mu.Unlock()

	wt := computeWebsiteToken(token, time.Now())

	u, err := url.Parse(g.apiURL() + "/contents/" + contentID)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("wt", wt)
	if password != "" {
		q.Set("password", password)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Website-Token", wt)
	req.Header.Set("X-BL", "en-US")
	req.AddCookie(&http.Cookie{Name: "accountToken", Value: token})

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("gofile: %w", site.ErrRateLimited)
	}

	var result contentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("gofile: decode response: %w", err)
	}

	switch result.Status {
	case "ok":
	case "error-notFound":
		return nil, fmt.Errorf("gofile: %w: %s", site.ErrNotFound, contentID)
	case "error-passwordRequired":
		return nil, fmt.Errorf("gofile: %w: password required", site.ErrAuthRequired)
	case "error-passwordIncorrect":
		return nil, fmt.Errorf("gofile: %w: incorrect password", site.ErrAuthRequired)
	case "error-notPremium":
		// Invalidate cached token and report clearly
		g.mu.Lock()
		g.accountToken = ""
		g.mu.Unlock()
		_ = removeCachedToken()
		return nil, fmt.Errorf("gofile: %w: API requires a premium account or valid MSD_GOFILE_TOKEN", site.ErrAuthRequired)
	case "error-rateLimit":
		return nil, fmt.Errorf("gofile: %w", site.ErrRateLimited)
	default:
		return nil, fmt.Errorf("gofile: API error: %s", result.Status)
	}

	if result.Data.Password && result.Data.PasswordStatus == "passwordWrong" {
		return nil, fmt.Errorf("gofile: %w: incorrect password", site.ErrAuthRequired)
	}
	if result.Data.PasswordStatus == "passwordRequired" {
		return nil, fmt.Errorf("gofile: %w: password required", site.ErrAuthRequired)
	}

	return &result.Data, nil
}

// --- Website token computation (isolated for easy updates) ---

func computeWebsiteToken(accountToken string, now time.Time) string {
	timeSlot := now.Unix() / tokenWindow
	raw := userAgent + "::en-US::" + accountToken + "::" + strconv.FormatInt(timeSlot, 10) + "::" + tokenSeed
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", hash)
}

func envAccountToken() string {
	if token := strings.TrimSpace(os.Getenv("MSD_GOFILE_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("GOFILE_TOKEN"))
}

func parseContentID(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid URL")
	}
	if strings.ToLower(u.Hostname()) != "gofile.io" {
		return "", fmt.Errorf("unsupported host")
	}
	m := contentPattern.FindStringSubmatch(u.EscapedPath())
	if m == nil {
		return "", fmt.Errorf("invalid content URL")
	}
	return m[1], nil
}

// --- Token caching ---

func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "msd", "gofile_token.json"), nil
}

func loadCachedToken() (string, error) {
	path, err := tokenPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var cache struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		return "", err
	}
	return cache.Token, nil
}

func saveCachedToken(token string) error {
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(struct {
		Token string `json:"token"`
	}{Token: token})
	return os.WriteFile(path, data, 0o600)
}

func removeCachedToken() error {
	path, err := tokenPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// --- Helpers ---

func (g *Gofile) httpClient() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return http.DefaultClient
}

func (g *Gofile) apiURL() string {
	if g.APIURL != "" {
		return strings.TrimRight(g.APIURL, "/")
	}
	return defaultAPIURL
}

// --- API types ---

type contentsResponse struct {
	Status string       `json:"status"`
	Data   contentsData `json:"data"`
}

type contentsData struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Type           string                   `json:"type"`
	Size           int64                    `json:"size"`
	Link           string                   `json:"link"`
	Children       map[string]contentsChild `json:"children"`
	Password       bool                     `json:"password"`
	PasswordStatus string                   `json:"passwordStatus"`
}

type contentsChild struct {
	ID       string                   `json:"id"`
	Name     string                   `json:"name"`
	Type     string                   `json:"type"`
	Size     int64                    `json:"size"`
	Link     string                   `json:"link"`
	Children map[string]contentsChild `json:"children"`
}
