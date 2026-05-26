package site

import (
	"context"
	"errors"
	"net/http"
	"time"
)

var (
	ErrSiteChanged  = errors.New("site structure changed")
	ErrRateLimited  = errors.New("rate limited")
	ErrAuthRequired = errors.New("authentication required")
	ErrNotFound     = errors.New("album or file not found")
)

func IsNotFound(err error) bool     { return errors.Is(err, ErrNotFound) }
func IsAuthRequired(err error) bool { return errors.Is(err, ErrAuthRequired) }
func IsRateLimited(err error) bool  { return errors.Is(err, ErrRateLimited) }

type File struct {
	ID   string
	Name string
	Size int64 // -1 if unknown
}

type Album struct {
	ID    string
	Name  string
	Files []File
}

type DownloadRequest struct {
	URL     string
	Headers http.Header
	Cookies []*http.Cookie
}

type Site interface {
	Name() string
	Match(url string) bool
	Resolve(ctx context.Context, url string, password string) (*Album, error)
	DownloadRequest(ctx context.Context, file File) (*DownloadRequest, error)
	DefaultConcurrency() int
	DefaultResolveDelay() time.Duration
	DefaultDownloadDelay() time.Duration
}
