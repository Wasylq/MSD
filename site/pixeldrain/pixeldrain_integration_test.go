//go:build integration

package pixeldrain

import (
	"context"
	"testing"
	"time"
)

const testAlbumURL = "https://pixeldrain.com/l/VVWU6TMC"

func TestIntegration_Resolve(t *testing.T) {
	p := &Pixeldrain{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	album, err := p.Resolve(ctx, testAlbumURL, "")
	if err != nil {
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

	for _, f := range album.Files {
		if f.ID == "" {
			t.Error("file ID is empty")
		}
		if f.Name == "" {
			t.Error("file Name is empty")
		}
		t.Logf("  %s (%d bytes)", f.Name, f.Size)
	}
}
