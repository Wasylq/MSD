package site

import "sync"

var (
	mu    sync.RWMutex
	sites []Site
)

func Register(s Site) {
	mu.Lock()
	defer mu.Unlock()
	sites = append(sites, s)
}

func Match(url string) Site {
	mu.RLock()
	defer mu.RUnlock()
	for _, s := range sites {
		if s.Match(url) {
			return s
		}
	}
	return nil
}

func List() []Site {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]Site, len(sites))
	copy(result, sites)
	return result
}
