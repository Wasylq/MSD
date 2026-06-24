//go:build integration

package turbo

import (
	"context"
	"strings"
	"testing"
)

const testAlbumURL = "https://turbo.cr/a/_mLcPZ7SPCu"

func TestIntegration_ResolveAndSign(t *testing.T) {
	ctx := context.Background()
	tr := &Turbo{}

	album, err := tr.Resolve(ctx, testAlbumURL, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if album.ID != "_mLcPZ7SPCu" {
		t.Errorf("album ID = %q", album.ID)
	}
	if len(album.Files) == 0 {
		t.Fatal("expected files")
	}

	req, err := tr.DownloadRequest(ctx, album.Files[0])
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if !strings.Contains(req.URL, "turbocdn") {
		t.Errorf("download URL = %q", req.URL)
	}
}
