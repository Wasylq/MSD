package engine

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Wasylq/MSD/site"
)

func (e *Engine) downloadFile(ctx context.Context, s site.Site, file site.File, dir string) error {
	destPath := filepath.Join(dir, file.Name)
	partPath := destPath + ".part"

	if info, err := os.Stat(destPath); err == nil {
		if file.Size <= 0 || info.Size() == file.Size {
			log.Printf("skip complete: %s (%d bytes)", destPath, info.Size())
			e.progress().OnFileStart(file)
			e.progress().OnFileProgress(file, info.Size(), file.Size)
			e.progress().OnFileComplete(file, nil)
			return nil
		}
	}

	e.progress().OnFileStart(file)
	log.Printf("download start: %s -> %s", file.ID, destPath)

	err := e.Retry.Do(ctx, func() error {
		return e.doDownload(ctx, s, file, destPath, partPath)
	})
	if err != nil {
		log.Printf("download failed: %s: %v", file.Name, err)
	} else {
		log.Printf("download complete: %s", file.Name)
	}

	e.progress().OnFileComplete(file, err)
	return err
}

func (e *Engine) doDownload(ctx context.Context, s site.Site, file site.File, destPath, partPath string) error {
	dlReq, err := s.DownloadRequest(ctx, file)
	if err != nil {
		return err
	}
	log.Printf("download request: %s -> %s", file.Name, dlReq.URL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlReq.URL, nil)
	if err != nil {
		return err
	}
	for k, v := range dlReq.Headers {
		req.Header[k] = v
	}
	for _, c := range dlReq.Cookies {
		req.AddCookie(c)
	}

	var offset int64
	if !e.NoResume {
		if info, err := os.Stat(partPath); err == nil && info.Size() > 0 {
			offset = info.Size()
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
			log.Printf("resume: %s from byte %d", file.Name, offset)
		}
	}

	resp, err := e.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	log.Printf("download response: %s: %d %s", file.Name, resp.StatusCode, resp.Status)

	switch resp.StatusCode {
	case http.StatusOK:
		offset = 0
	case http.StatusPartialContent:
		// resume successful, offset stays
	case http.StatusRequestedRangeNotSatisfiable:
		os.Remove(partPath)
		return errRangeReset
	case http.StatusTooManyRequests:
		return site.ErrRateLimited
	case http.StatusNotFound:
		return site.ErrNotFound
	default:
		if resp.StatusCode >= 400 {
			return &HTTPError{StatusCode: resp.StatusCode}
		}
	}

	flag := os.O_WRONLY | os.O_CREATE
	if offset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(partPath, flag, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	written, err := e.copyWithProgress(f, resp.Body, file, offset)
	if err != nil {
		return err
	}

	if file.Size > 0 && offset+written != file.Size {
		return fmt.Errorf("size mismatch: expected %d, got %d", file.Size, offset+written)
	}

	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	return os.Rename(partPath, destPath)
}

func (e *Engine) copyWithProgress(dst io.Writer, src io.Reader, file site.File, offset int64) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		nr, readErr := src.Read(buf)
		if nr > 0 {
			nw, writeErr := dst.Write(buf[:nr])
			if writeErr != nil {
				return written, writeErr
			}
			written += int64(nw)
			e.progress().OnFileProgress(file, offset+written, file.Size)
		}
		if readErr != nil {
			if readErr == io.EOF {
				return written, nil
			}
			return written, readErr
		}
	}
}
