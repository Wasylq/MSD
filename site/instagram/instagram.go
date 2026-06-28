package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wasylq/MSD/site"
)

const (
	defaultBaseURL       = "https://www.instagram.com"
	profileInfoPath      = "/api/v1/users/web_profile_info/"
	timelineQueryID      = "7950326061742207"
	webAppID             = "936619743392459"
	userAgent            = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
	pageUserAgent        = "Mozilla/5.0"
	timelinePageSize     = 12
	maxTimelinePageCount = 200
)

func init() { site.Register(&Instagram{}) }

type Instagram struct {
	HTTPClient *http.Client
	BaseURL    string

	mu    sync.Mutex
	links map[string]string
}

func (i *Instagram) Name() string { return "instagram" }

func (i *Instagram) Match(rawURL string) bool {
	return parseProfileURL(rawURL) != "" || parseReelURL(rawURL) != ""
}

func (i *Instagram) Resolve(ctx context.Context, rawURL string, _ string) (*site.Album, error) {
	if shortcode := parseReelURL(rawURL); shortcode != "" {
		return i.resolveReel(ctx, shortcode)
	}

	username := parseProfileURL(rawURL)
	if username == "" {
		return nil, fmt.Errorf("instagram: %w: %s", site.ErrNotFound, rawURL)
	}
	return i.resolveProfile(ctx, username)
}

func (i *Instagram) resolveProfile(ctx context.Context, username string) (*site.Album, error) {
	profile, err := i.fetchProfile(ctx, username)
	if err != nil {
		return nil, err
	}
	if profile.Data.User == nil || profile.Data.User.ID == "" {
		return nil, fmt.Errorf("instagram: %w: %s", site.ErrNotFound, username)
	}
	user := profile.Data.User
	if user.IsPrivate {
		return nil, fmt.Errorf("instagram: %w: private profile", site.ErrAuthRequired)
	}
	if user.Username != "" {
		username = user.Username
	}

	i.resetLinks()

	album := &site.Album{
		ID:   user.ID,
		Name: username,
	}
	dateCounts := make(map[string]int)
	seenPosts := make(map[string]struct{})

	conn := user.timeline()
	i.addConnection(album, conn, dateCounts, seenPosts)

	for page := 0; conn.PageInfo.HasNextPage && conn.PageInfo.EndCursor != ""; page++ {
		if page >= maxTimelinePageCount {
			return nil, fmt.Errorf("instagram: %w: pagination exceeded %d pages", site.ErrSiteChanged, maxTimelinePageCount)
		}
		next, err := i.fetchTimelinePage(ctx, user.ID, conn.PageInfo.EndCursor)
		if err != nil {
			return nil, err
		}
		conn = next.Data.User.EdgeOwnerToTimelineMedia
		i.addConnection(album, conn, dateCounts, seenPosts)
	}

	if len(album.Files) == 0 {
		return nil, fmt.Errorf("instagram: %w: no downloadable files for %s", site.ErrNotFound, username)
	}

	return album, nil
}

func (i *Instagram) resolveReel(ctx context.Context, shortcode string) (*site.Album, error) {
	username, err := i.fetchReelOwner(ctx, shortcode)
	if err != nil {
		return nil, err
	}
	profile, err := i.fetchProfile(ctx, username)
	if err != nil {
		return nil, err
	}
	if profile.Data.User == nil || profile.Data.User.ID == "" {
		return nil, fmt.Errorf("instagram: %w: %s", site.ErrNotFound, username)
	}
	user := profile.Data.User
	if user.IsPrivate {
		return nil, fmt.Errorf("instagram: %w: private profile", site.ErrAuthRequired)
	}
	if user.Username != "" {
		username = user.Username
	}

	i.resetLinks()

	album := &site.Album{
		ID:   user.ID,
		Name: username,
	}
	dateCounts := make(map[string]int)
	seenPosts := make(map[string]struct{})

	conn := user.timeline()
	found := i.addMatchingReel(album, conn, dateCounts, seenPosts, shortcode)
	for page := 0; !found && conn.PageInfo.HasNextPage && conn.PageInfo.EndCursor != ""; page++ {
		if page >= maxTimelinePageCount {
			return nil, fmt.Errorf("instagram: %w: pagination exceeded %d pages", site.ErrSiteChanged, maxTimelinePageCount)
		}
		next, err := i.fetchTimelinePage(ctx, user.ID, conn.PageInfo.EndCursor)
		if err != nil {
			return nil, err
		}
		conn = next.Data.User.EdgeOwnerToTimelineMedia
		found = i.addMatchingReel(album, conn, dateCounts, seenPosts, shortcode)
	}
	if !found {
		return nil, fmt.Errorf("instagram: %w: reel %s was not found in %s", site.ErrNotFound, shortcode, username)
	}
	if len(album.Files) == 0 {
		return nil, fmt.Errorf("instagram: %w: no downloadable files for reel %s", site.ErrNotFound, shortcode)
	}
	return album, nil
}

func (i *Instagram) addMatchingReel(album *site.Album, conn mediaConnection, dateCounts map[string]int, seenPosts map[string]struct{}, shortcode string) bool {
	for _, edge := range conn.Edges {
		if edge.Node.shortcode() == shortcode {
			i.addPost(album, edge.Node, dateCounts, seenPosts, "reel", "/reel/", shortcode)
			return true
		}
	}
	return false
}

func (i *Instagram) addConnection(album *site.Album, conn mediaConnection, dateCounts map[string]int, seenPosts map[string]struct{}) {
	for _, edge := range conn.Edges {
		i.addPost(album, edge.Node, dateCounts, seenPosts, "", "/p/", "")
	}
}

func (i *Instagram) addPost(album *site.Album, post mediaNode, dateCounts map[string]int, seenPosts map[string]struct{}, nameMarker, linkPath, shortcodeFallback string) {
	postID := post.mediaID(shortcodeFallback)
	if postID == "" {
		return
	}
	if _, ok := seenPosts[postID]; ok {
		return
	}
	seenPosts[postID] = struct{}{}

	shortcode := post.shortcode()
	if shortcode == "" {
		shortcode = shortcodeFallback
	}
	if shortcode != "" {
		album.PostLinks = append(album.PostLinks, i.baseURL()+linkPath+shortcode+"/")
	}

	date := postDate(post.timestamp())
	media := post.mediaItems()
	for index, item := range media {
		link := item.downloadURL()
		if link == "" {
			continue
		}
		ext := item.extension()
		dateCounts[date]++
		id := postID + ":" + item.mediaID(strconv.Itoa(index+1))
		name := fileName(date, nameMarker, dateCounts[date], ext)

		i.mu.Lock()
		i.links[id] = link
		i.mu.Unlock()

		album.Files = append(album.Files, site.File{
			ID:   id,
			Name: name,
			Size: -1,
		})
	}
}

func (i *Instagram) resetLinks() {
	i.mu.Lock()
	i.links = make(map[string]string)
	i.mu.Unlock()
}

func (i *Instagram) fetchProfile(ctx context.Context, username string) (*profileResponse, error) {
	u, err := url.Parse(i.baseURL() + profileInfoPath)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("username", username)
	u.RawQuery = q.Encode()

	var result profileResponse
	if err := i.getJSON(ctx, u.String(), &result); err != nil {
		return nil, err
	}
	if result.Status != "" && result.Status != "ok" {
		return nil, fmt.Errorf("instagram: API status %s", result.Status)
	}
	return &result, nil
}

func (i *Instagram) fetchTimelinePage(ctx context.Context, userID, after string) (*timelineResponse, error) {
	variables, err := json.Marshal(map[string]any{
		"id":                             userID,
		"first":                          timelinePageSize,
		"after":                          after,
		"include_clips_attribution_info": false,
	})
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(i.baseURL() + "/graphql/query/")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query_id", timelineQueryID)
	q.Set("variables", string(variables))
	u.RawQuery = q.Encode()

	var result timelineResponse
	if err := i.getJSON(ctx, u.String(), &result); err != nil {
		return nil, err
	}
	if result.Status != "" && result.Status != "ok" {
		return nil, fmt.Errorf("instagram: API status %s", result.Status)
	}
	if result.Data.User == nil {
		return nil, fmt.Errorf("instagram: %w: missing user in timeline response", site.ErrSiteChanged)
	}
	return &result, nil
}

func (i *Instagram) getJSON(ctx context.Context, apiURL string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-IG-App-ID", webAppID)
	req.Header.Set("Referer", i.baseURL()+"/")

	resp, err := i.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return fmt.Errorf("instagram: %w: %s", site.ErrNotFound, apiURL)
	case http.StatusTooManyRequests:
		return fmt.Errorf("instagram: %w", site.ErrRateLimited)
	case http.StatusForbidden, http.StatusUnauthorized:
		return fmt.Errorf("instagram: %w: API denied", site.ErrAuthRequired)
	default:
		if resp.StatusCode >= 400 {
			return fmt.Errorf("instagram: unexpected status %d for %s", resp.StatusCode, apiURL)
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("instagram: decode response: %w", err)
	}
	return nil
}

func (i *Instagram) fetchReelOwner(ctx context.Context, shortcode string) (string, error) {
	pageURL := i.baseURL() + "/reel/" + url.PathEscape(shortcode) + "/"
	page, err := i.getText(ctx, pageURL)
	if err != nil {
		return "", err
	}
	username := extractReelOwner(page, shortcode)
	if username != "" {
		return username, nil
	}
	ownerID := extractReelOwnerID(page)
	if ownerID != "" {
		return i.fetchUsernameByID(ctx, ownerID)
	}
	return "", fmt.Errorf("instagram: %w: missing reel owner for %s", site.ErrSiteChanged, shortcode)
}

func (i *Instagram) fetchUsernameByID(ctx context.Context, userID string) (string, error) {
	u, err := url.Parse(i.baseURL() + "/api/v1/users/" + url.PathEscape(userID) + "/info/")
	if err != nil {
		return "", err
	}

	var result userInfoResponse
	if err := i.getJSON(ctx, u.String(), &result); err != nil {
		return "", err
	}
	if result.Status != "" && result.Status != "ok" {
		return "", fmt.Errorf("instagram: API status %s", result.Status)
	}
	if result.User == nil || result.User.Username == "" {
		return "", fmt.Errorf("instagram: %w: missing username for user %s", site.ErrSiteChanged, userID)
	}
	return result.User.Username, nil
}

func (i *Instagram) getText(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", pageUserAgent)

	resp, err := i.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", fmt.Errorf("instagram: %w: %s", site.ErrNotFound, pageURL)
	case http.StatusTooManyRequests:
		return "", fmt.Errorf("instagram: %w", site.ErrRateLimited)
	case http.StatusForbidden, http.StatusUnauthorized:
		return "", fmt.Errorf("instagram: %w: page denied", site.ErrAuthRequired)
	default:
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("instagram: unexpected status %d for %s", resp.StatusCode, pageURL)
		}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (i *Instagram) DownloadRequest(_ context.Context, file site.File) (*site.DownloadRequest, error) {
	i.mu.Lock()
	link := i.links[file.ID]
	i.mu.Unlock()
	if link == "" {
		return nil, fmt.Errorf("instagram: no download link for %s", file.ID)
	}
	return &site.DownloadRequest{
		URL: link,
		Headers: http.Header{
			"User-Agent": {userAgent},
			"Referer":    {i.baseURL() + "/"},
		},
	}, nil
}

func (i *Instagram) DefaultConcurrency() int             { return 2 }
func (i *Instagram) DefaultResolveDelay() time.Duration  { return 2 * time.Second }
func (i *Instagram) DefaultDownloadDelay() time.Duration { return 2 * time.Second }

func (i *Instagram) httpClient() *http.Client {
	if i.HTTPClient != nil {
		return i.HTTPClient
	}
	return http.DefaultClient
}

func (i *Instagram) baseURL() string {
	if i.BaseURL != "" {
		return strings.TrimRight(i.BaseURL, "/")
	}
	return defaultBaseURL
}

func parseProfileURL(rawURL string) string {
	parts, ok := instagramPathParts(rawURL)
	if !ok {
		return ""
	}
	if len(parts) != 1 || parts[0] == "" {
		return ""
	}
	username, err := url.PathUnescape(parts[0])
	if err != nil {
		return ""
	}
	reserved := map[string]struct{}{
		"about": {}, "accounts": {}, "api": {}, "developer": {}, "direct": {}, "explore": {}, "p": {}, "reel": {}, "reels": {}, "stories": {},
	}
	if _, ok := reserved[strings.ToLower(username)]; ok {
		return ""
	}
	return username
}

func parseReelURL(rawURL string) string {
	parts, ok := instagramPathParts(rawURL)
	if !ok {
		return ""
	}
	if len(parts) != 2 || (strings.ToLower(parts[0]) != "reel" && strings.ToLower(parts[0]) != "reels") || parts[1] == "" {
		return ""
	}
	shortcode, err := url.PathUnescape(parts[1])
	if err != nil {
		return ""
	}
	return shortcode
}

func instagramPathParts(rawURL string) ([]string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, false
	}
	host := strings.ToLower(u.Hostname())
	if host != "instagram.com" && host != "www.instagram.com" {
		return nil, false
	}
	return strings.Split(strings.Trim(u.Path, "/"), "/"), true
}

func fileName(date, marker string, number int, ext string) string {
	if marker != "" {
		return fmt.Sprintf("%s_%s_%d%s", date, marker, number, ext)
	}
	return fmt.Sprintf("%s_%d%s", date, number, ext)
}

func extractReelOwner(page, shortcode string) string {
	for _, re := range reelURLPatterns {
		for _, match := range re.FindAllStringSubmatch(page, -1) {
			if username := ownerFromReelURL(html.UnescapeString(match[1]), shortcode); username != "" {
				return username
			}
		}
	}
	return ""
}

func extractReelOwnerID(page string) string {
	match := reelOwnerIDPattern.FindStringSubmatch(page)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

var reelURLPatterns = []*regexp.Regexp{
	regexp.MustCompile(`<meta\s+[^>]*property=["']og:url["'][^>]*content=["']([^"']+)["']`),
	regexp.MustCompile(`<meta\s+[^>]*content=["']([^"']+)["'][^>]*property=["']og:url["']`),
}

var reelOwnerIDPattern = regexp.MustCompile(`"owner_id":"([0-9]+)"`)

func ownerFromReelURL(rawURL, shortcode string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || strings.ToLower(parts[1]) != "reel" || parts[2] != shortcode {
		return ""
	}
	username, err := url.PathUnescape(parts[0])
	if err != nil {
		return ""
	}
	return username
}

func postDate(ts int64) string {
	if ts <= 0 {
		return "unknown"
	}
	return time.Unix(ts, 0).UTC().Format("060102")
}

type profileResponse struct {
	Data struct {
		User *userResponse `json:"user"`
	} `json:"data"`
	Status string `json:"status"`
}

type timelineResponse struct {
	Data struct {
		User *userResponse `json:"user"`
	} `json:"data"`
	Status string `json:"status"`
}

type userInfoResponse struct {
	User *struct {
		ID       string `json:"id"`
		PK       string `json:"pk"`
		Username string `json:"username"`
	} `json:"user"`
	Status string `json:"status"`
}

type userResponse struct {
	ID                        string          `json:"id"`
	Username                  string          `json:"username"`
	IsPrivate                 bool            `json:"is_private"`
	EdgeOwnerToTimelineMedia  mediaConnection `json:"edge_owner_to_timeline_media"`
	XDTUserTimelineConnection mediaConnection `json:"xdt_api__v1__feed__user_timeline_graphql_connection"`
}

func (u userResponse) timeline() mediaConnection {
	if len(u.EdgeOwnerToTimelineMedia.Edges) > 0 || u.EdgeOwnerToTimelineMedia.Count > 0 {
		return u.EdgeOwnerToTimelineMedia
	}
	return u.XDTUserTimelineConnection
}

type mediaConnection struct {
	Count    int         `json:"count"`
	PageInfo pageInfo    `json:"page_info"`
	Edges    []mediaEdge `json:"edges"`
}

type pageInfo struct {
	HasNextPage bool   `json:"has_next_page"`
	EndCursor   string `json:"end_cursor"`
}

type mediaEdge struct {
	Node mediaNode `json:"node"`
}

type mediaNode struct {
	Typename              string          `json:"__typename"`
	ID                    string          `json:"id"`
	PK                    string          `json:"pk"`
	Code                  string          `json:"code"`
	Shortcode             string          `json:"shortcode"`
	DisplayURL            string          `json:"display_url"`
	VideoURL              string          `json:"video_url"`
	IsVideo               bool            `json:"is_video"`
	MediaType             int             `json:"media_type"`
	TakenAtTimestamp      int64           `json:"taken_at_timestamp"`
	TakenAt               int64           `json:"taken_at"`
	EdgeSidecarToChildren mediaConnection `json:"edge_sidecar_to_children"`
	CarouselMedia         []mediaNode     `json:"carousel_media"`
	DisplayResources      []imageResource `json:"display_resources"`
	ImageVersions2        imageVersions   `json:"image_versions2"`
	VideoVersions         []imageResource `json:"video_versions"`
	User                  *mediaUser      `json:"user"`
}

type mediaUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type imageVersions struct {
	Candidates []imageResource `json:"candidates"`
}

type imageResource struct {
	Src    string `json:"src"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (m mediaNode) mediaItems() []mediaNode {
	if len(m.EdgeSidecarToChildren.Edges) > 0 {
		items := make([]mediaNode, 0, len(m.EdgeSidecarToChildren.Edges))
		for _, edge := range m.EdgeSidecarToChildren.Edges {
			child := edge.Node
			if child.TakenAtTimestamp == 0 {
				child.TakenAtTimestamp = m.TakenAtTimestamp
			}
			if child.TakenAt == 0 {
				child.TakenAt = m.TakenAt
			}
			items = append(items, child)
		}
		return items
	}
	if len(m.CarouselMedia) > 0 {
		items := make([]mediaNode, 0, len(m.CarouselMedia))
		for _, child := range m.CarouselMedia {
			if child.TakenAtTimestamp == 0 {
				child.TakenAtTimestamp = m.TakenAtTimestamp
			}
			if child.TakenAt == 0 {
				child.TakenAt = m.TakenAt
			}
			items = append(items, child)
		}
		return items
	}
	return []mediaNode{m}
}

func (m mediaNode) downloadURL() string {
	if m.isVideo() {
		if m.VideoURL != "" {
			return m.VideoURL
		}
		if link := bestResourceURL(m.VideoVersions); link != "" {
			return link
		}
	}
	if m.DisplayURL != "" {
		return m.DisplayURL
	}
	if link := bestResourceURL(m.ImageVersions2.Candidates); link != "" {
		return link
	}
	return bestResourceURL(m.DisplayResources)
}

func (m mediaNode) extension() string {
	link := m.downloadURL()
	if u, err := url.Parse(link); err == nil {
		if ext := path.Ext(u.Path); ext != "" {
			return ext
		}
	}
	if m.isVideo() {
		return ".mp4"
	}
	return ".jpg"
}

func (m mediaNode) mediaID(fallback string) string {
	if m.ID != "" {
		return m.ID
	}
	if m.PK != "" {
		return m.PK
	}
	return fallback
}

func (m mediaNode) shortcode() string {
	if m.Shortcode != "" {
		return m.Shortcode
	}
	return m.Code
}

func (m mediaNode) isVideo() bool {
	return m.IsVideo || m.MediaType == 2 || strings.Contains(strings.ToLower(m.Typename), "video")
}

func (m mediaNode) timestamp() int64 {
	if m.TakenAtTimestamp != 0 {
		return m.TakenAtTimestamp
	}
	return m.TakenAt
}

func bestResourceURL(resources []imageResource) string {
	bestURL := ""
	bestArea := -1
	for _, resource := range resources {
		link := resource.URL
		if link == "" {
			link = resource.Src
		}
		if link == "" {
			continue
		}
		area := resource.Width * resource.Height
		if area >= bestArea {
			bestArea = area
			bestURL = link
		}
	}
	return bestURL
}
