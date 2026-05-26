package engine

import "github.com/Wasylq/MSD/site"

type ProgressReporter interface {
	OnFileStart(file site.File)
	OnFileProgress(file site.File, bytesDownloaded, totalBytes int64)
	OnFileComplete(file site.File, err error)
	OnAlbumComplete(album site.Album, succeeded, failed int)
}

type NoopReporter struct{}

func (NoopReporter) OnFileStart(site.File)                  {}
func (NoopReporter) OnFileProgress(site.File, int64, int64) {}
func (NoopReporter) OnFileComplete(site.File, error)        {}
func (NoopReporter) OnAlbumComplete(site.Album, int, int)   {}
