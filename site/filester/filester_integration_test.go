//go:build integration

package filester

import (
	"context"
	"testing"
	"time"
)

const testAlbumURL = "https://filester.me/f/3de7fbc9228bb07f"

func TestIntegration_Resolve(t *testing.T) {
	f := &Filester{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	album, err := f.Resolve(ctx, testAlbumURL, "")
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

	for _, file := range album.Files {
		if file.ID == "" {
			t.Error("file ID is empty")
		}
		if file.Name == "" {
			t.Error("file Name is empty")
		}
		t.Logf("  %s (slug=%s, size=%d)", file.Name, file.ID, file.Size)
	}
}

func TestIntegration_DownloadRequest(t *testing.T) {
	f := &Filester{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	album, err := f.Resolve(ctx, testAlbumURL, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(album.Files) == 0 {
		t.Fatal("no files to test download request")
	}

	req, err := f.DownloadRequest(ctx, album.Files[0])
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}

	if req.URL == "" {
		t.Error("download URL is empty")
	}
	t.Logf("CDN URL: %s", req.URL)
}
