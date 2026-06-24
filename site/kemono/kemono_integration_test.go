//go:build integration

package kemono

import (
	"context"
	"strings"
	"testing"
	"time"
)

const testPawchiveURL = "https://pawchive.st/patreon/user/59577203"

func TestIntegration_ResolvePawchive(t *testing.T) {
	k := &Kemono{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	album, err := k.Resolve(ctx, testPawchiveURL, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if album.ID != "patreon-59577203" {
		t.Errorf("album ID = %q, want patreon-59577203", album.ID)
	}
	if album.Name == "" {
		t.Error("album Name is empty")
	}
	if len(album.Files) == 0 {
		t.Fatal("album has no files")
	}
	if len(album.PostLinks) == 0 {
		t.Fatal("album has no post links")
	}
	if !strings.HasPrefix(album.PostLinks[0], testPawchiveURL+"/post/") {
		t.Errorf("first post link = %q", album.PostLinks[0])
	}

	req, err := k.DownloadRequest(ctx, album.Files[0])
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if !strings.HasPrefix(req.URL, "https://file.pawchive.st/data/") {
		t.Errorf("download URL = %q", req.URL)
	}
}
