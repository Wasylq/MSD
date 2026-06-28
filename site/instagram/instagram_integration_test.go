//go:build integration

package instagram

import (
	"context"
	"testing"
	"time"
)

const testProfileURL = "https://www.instagram.com/salmahayek/"

func TestIntegration_Resolve(t *testing.T) {
	i := &Instagram{}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	album, err := i.Resolve(ctx, testProfileURL, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if album.ID == "" {
		t.Error("album ID is empty")
	}
	if album.Name != "salmahayek" {
		t.Errorf("album Name = %q, want salmahayek", album.Name)
	}
	if len(album.Files) == 0 {
		t.Fatal("album has no files")
	}
	if len(album.PostLinks) == 0 {
		t.Fatal("album has no post links")
	}
	if !hasDatePrefix(album.Files[0].Name) {
		t.Errorf("first file name does not start with YYMMDD date: %q", album.Files[0].Name)
	}

	req, err := i.DownloadRequest(ctx, album.Files[0])
	if err != nil {
		t.Fatalf("DownloadRequest: %v", err)
	}
	if req.URL == "" {
		t.Error("download URL is empty")
	}
}

func hasDatePrefix(name string) bool {
	if len(name) < len("060102_") || name[6] != '_' {
		return false
	}
	for _, c := range name[:6] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
