//go:build integration

package bunkr

import (
	"context"
	"strings"
	"testing"
)

const testAlbumURL = "https://bunkr.cr/a/Z64Hzaqy"

func TestIntegration_ResolveAndSign(t *testing.T) {
	ctx := context.Background()
	b := &Bunkr{}

	album, err := b.Resolve(ctx, testAlbumURL, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if album.ID != "Z64Hzaqy" {
		t.Errorf("album ID = %q", album.ID)
	}
	if len(album.Files) == 0 {
		t.Fatal("expected files")
	}

	req, err := b.DownloadRequest(ctx, album.Files[0])
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if !strings.Contains(req.URL, "token=") || !strings.Contains(req.URL, "ex=") {
		t.Errorf("download URL is not signed: %q", req.URL)
	}
}
