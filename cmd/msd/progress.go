package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/schollz/progressbar/v3"

	"github.com/Wasylq/MSD/site"
)

type progressReporter struct {
	mu   sync.Mutex
	bars map[string]*progressbar.ProgressBar
}

func newProgressReporter(album *site.Album) *progressReporter {
	return &progressReporter{
		bars: make(map[string]*progressbar.ProgressBar, len(album.Files)),
	}
}

func (p *progressReporter) OnFileStart(file site.File) {
	max := int64(-1)
	if file.Size > 0 {
		max = file.Size
	}

	bar := progressbar.NewOptions64(max,
		progressbar.OptionSetDescription(truncateName(file.Name, 40)),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() { fmt.Fprintln(os.Stderr) }),
		progressbar.OptionSetWidth(30),
		progressbar.OptionThrottle(100*1e6), // 100ms
		progressbar.OptionSpinnerType(14),
	)

	p.mu.Lock()
	p.bars[file.ID] = bar
	p.mu.Unlock()
}

func (p *progressReporter) OnFileProgress(file site.File, downloaded, total int64) {
	p.mu.Lock()
	bar := p.bars[file.ID]
	p.mu.Unlock()
	if bar == nil {
		return
	}
	_ = bar.Set64(downloaded)
}

func (p *progressReporter) OnFileComplete(file site.File, err error) {
	p.mu.Lock()
	bar := p.bars[file.ID]
	delete(p.bars, file.ID)
	p.mu.Unlock()
	if bar == nil {
		return
	}

	if err != nil {
		bar.Describe(truncateName(file.Name, 40) + " FAILED")
		_ = bar.Finish()
		fmt.Fprintf(os.Stderr, "%s: %v\n", file.Name, err)
	} else {
		_ = bar.Finish()
	}
}

func (p *progressReporter) OnAlbumComplete(album site.Album, succeeded, failed int) {
	fmt.Fprintf(os.Stderr, "\nDone: %d succeeded, %d failed\n", succeeded, failed)
}

func (p *progressReporter) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, bar := range p.bars {
		_ = bar.Finish()
	}
}

func truncateName(name string, max int) string {
	if len(name) <= max {
		return name
	}
	return name[:max-3] + "..."
}
