//go:build integration

package gofile

import (
	"context"
	"testing"
	"time"

	"github.com/Wasylq/MSD/site"
)

const testAlbumURL = "https://gofile.io/d/5cXXCq"

func TestIntegration_Resolve(t *testing.T) {
	if envAccountToken() == "" {
		t.Skip("set MSD_GOFILE_TOKEN or GOFILE_TOKEN to run live Gofile integration tests")
	}

	g := &Gofile{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	album, err := g.Resolve(ctx, testAlbumURL, "")
	if err != nil {
		if site.IsAuthRequired(err) {
			t.Skipf("Gofile requires a valid account token for this content: %v", err)
		}
		t.Fatalf("Resolve: %v", err)
	}

	if album.ID == "" {
		t.Error("album ID is empty")
	}
	if album.Name == "" {
		t.Error("album Name is empty")
	}
	if len(album.Files) == 0 {
		t.Error("album has no files")
	}

	for _, file := range album.Files {
		if file.ID == "" {
			t.Error("file ID is empty")
		}
		if file.Name == "" {
			t.Error("file Name is empty")
		}
		t.Logf("  %s (%d bytes)", file.Name, file.Size)
	}

	// Verify download links are populated
	g.mu.Lock()
	for _, file := range album.Files {
		if g.links[file.ID] == "" {
			t.Errorf("no download link for %s", file.Name)
		}
	}
	g.mu.Unlock()
}
