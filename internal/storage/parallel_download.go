/*
Copyright 2026 CKodex Authors.
Licensed under the Apache License, Version 2.0.
*/

package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultChunkSize is the default size of each download chunk (64 MB).
	DefaultChunkSize int64 = 64 * 1024 * 1024
	// DefaultDownloadWorkers is the default concurrency for parallel downloads.
	DefaultDownloadWorkers = 4
	// LargeFileThreshold is the minimum file size to trigger parallel chunk download (1 GB).
	LargeFileThreshold int64 = 1 * 1024 * 1024 * 1024
	// ProgressLogInterval is how often progress is logged.
	ProgressLogInterval = 5 * time.Second
)

// ParallelDownloader performs chunked parallel downloads with resume support.
type ParallelDownloader struct {
	Workers    int
	ChunkSize  int64
	HTTPClient *http.Client
	Token      string
}

// NewParallelDownloader creates a downloader with settings from environment.
func NewParallelDownloader(token string) *ParallelDownloader {
	workers := DefaultDownloadWorkers
	if v := os.Getenv("STORAGE_DOWNLOAD_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 32 {
			workers = n
		}
	}

	chunkSize := DefaultChunkSize
	if v := os.Getenv("STORAGE_CHUNK_SIZE_MB"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			chunkSize = n * 1024 * 1024
		}
	}

	return &ParallelDownloader{
		Workers:    workers,
		ChunkSize:  chunkSize,
		HTTPClient: http.DefaultClient,
		Token:      token,
	}
}

// chunkDescriptor describes one range chunk to download.
type chunkDescriptor struct {
	Index int
	Start int64
	End   int64 // inclusive
	Path  string
}

// DownloadFile downloads a single file, choosing parallel chunk download for
// files larger than LargeFileThreshold, otherwise a simple single-stream download.
// After assembly, if expectedSHA256 is non-empty, the file is verified.
func (d *ParallelDownloader) DownloadFile(ctx context.Context, url, destFile, expectedSHA256 string) error {
	// Probe the remote file size and range support.
	fileSize, supportsRange, err := d.probeFile(ctx, url)
	if err != nil {
		return fmt.Errorf("parallel download: probe failed for %s: %w", url, err)
	}

	if fileSize >= LargeFileThreshold && supportsRange {
		fmt.Printf("Large file detected (%d bytes), using %d parallel workers with %d MB chunks\n",
			fileSize, d.Workers, d.ChunkSize/(1024*1024))
		if err := d.downloadChunked(ctx, url, destFile, fileSize); err != nil {
			return err
		}
	} else {
		if err := d.downloadSimple(ctx, url, destFile); err != nil {
			return err
		}
	}

	// Verify if checksum provided.
	if expectedSHA256 != "" {
		if err := VerifyFile(destFile, expectedSHA256); err != nil {
			// Remove the corrupt file.
			os.Remove(destFile)
			return err
		}
		fmt.Printf("Checksum verified: %s\n", filepath.Base(destFile))
	}

	return nil
}

// probeFile issues a HEAD request to determine file size and range support.
func (d *ParallelDownloader) probeFile(ctx context.Context, url string) (int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, false, err
	}
	d.setAuth(req)

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("HEAD %s returned %s", url, resp.Status)
	}

	size := resp.ContentLength
	acceptRanges := strings.ToLower(resp.Header.Get("Accept-Ranges"))
	supportsRange := acceptRanges == "bytes" && size > 0

	return size, supportsRange, nil
}

// downloadChunked splits the file into chunks and downloads them in parallel.
func (d *ParallelDownloader) downloadChunked(ctx context.Context, url, destFile string, fileSize int64) error {
	if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
		return err
	}

	// Create chunk directory.
	chunksDir := destFile + ".chunks"
	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		return fmt.Errorf("failed to create chunks dir: %w", err)
	}

	// Build chunk descriptors.
	var chunks []chunkDescriptor
	for i, offset := int(0), int64(0); offset < fileSize; i, offset = i+1, offset+d.ChunkSize {
		end := offset + d.ChunkSize - 1
		if end >= fileSize {
			end = fileSize - 1
		}
		chunks = append(chunks, chunkDescriptor{
			Index: i,
			Start: offset,
			End:   end,
			Path:  filepath.Join(chunksDir, fmt.Sprintf("chunk_%05d", i)),
		})
	}

	// Filter out already-complete chunks (resume support).
	var toDownload []chunkDescriptor
	for _, c := range chunks {
		expectedSize := c.End - c.Start + 1
		if info, err := os.Stat(c.Path); err == nil && info.Size() == expectedSize {
			fmt.Printf("Resuming: chunk %d already complete, skipping\n", c.Index)
			continue
		}
		toDownload = append(toDownload, c)
	}

	if len(toDownload) == 0 {
		fmt.Printf("All %d chunks already downloaded, assembling...\n", len(chunks))
	} else {
		fmt.Printf("Downloading %d/%d chunks (%d already cached)\n",
			len(toDownload), len(chunks), len(chunks)-len(toDownload))

		// Progress tracking.
		var totalDownloaded atomic.Int64
		totalToDownload := int64(0)
		for _, c := range toDownload {
			totalToDownload += c.End - c.Start + 1
		}

		// Start progress logger.
		progressCtx, progressCancel := context.WithCancel(ctx)
		defer progressCancel()
		go func() {
			ticker := time.NewTicker(ProgressLogInterval)
			defer ticker.Stop()
			for {
				select {
				case <-progressCtx.Done():
					return
				case <-ticker.C:
					downloaded := totalDownloaded.Load()
					pct := float64(downloaded) / float64(totalToDownload) * 100
					fmt.Printf("Download progress: %.1f%% (%d / %d bytes)\n",
						pct, downloaded, totalToDownload)
				}
			}
		}()

		// Download chunks in parallel.
		sem := make(chan struct{}, d.Workers)
		var wg sync.WaitGroup
		errCh := make(chan error, len(toDownload))

		for _, chunk := range toDownload {
			wg.Add(1)
			go func(c chunkDescriptor) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				if err := d.downloadChunk(ctx, url, c, &totalDownloaded); err != nil {
					errCh <- fmt.Errorf("chunk %d: %w", c.Index, err)
				}
			}(chunk)
		}

		wg.Wait()
		progressCancel()
		close(errCh)

		// Collect errors.
		var errs []string
		for e := range errCh {
			errs = append(errs, e.Error())
		}
		if len(errs) > 0 {
			return fmt.Errorf("parallel download failed:\n  %s", strings.Join(errs, "\n  "))
		}
	}

	// Assemble chunks into the final file using atomic rename.
	tmpFile := destFile + ".tmp"
	if err := d.assembleChunks(chunks, tmpFile); err != nil {
		os.Remove(tmpFile)
		return err
	}

	// Atomic rename.
	if err := os.Rename(tmpFile, destFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to finalize file: %w", err)
	}

	// Clean up chunks directory.
	os.RemoveAll(chunksDir)

	fmt.Printf("Assembled %d chunks into %s\n", len(chunks), filepath.Base(destFile))
	return nil
}

// downloadChunk downloads a single range chunk to disk.
func (d *ParallelDownloader) downloadChunk(ctx context.Context, url string, c chunkDescriptor, counter *atomic.Int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	d.setAuth(req)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", c.Start, c.End))

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s for range %d-%d", resp.Status, c.Start, c.End)
	}

	// Write to a temporary file first, then rename for atomicity at the chunk level.
	tmpPath := c.Path + ".partial"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	// Wrap with a counting writer to track progress.
	written, err := io.Copy(f, &countingReader{r: resp.Body, counter: counter})
	f.Close()
	if err != nil {
		os.Remove(tmpPath)
		return err
	}

	expectedSize := c.End - c.Start + 1
	if written != expectedSize {
		os.Remove(tmpPath)
		return fmt.Errorf("short write: got %d, expected %d", written, expectedSize)
	}

	return os.Rename(tmpPath, c.Path)
}

// assembleChunks concatenates chunk files into a single output file.
func (d *ParallelDownloader) assembleChunks(chunks []chunkDescriptor, destFile string) error {
	out, err := os.Create(destFile)
	if err != nil {
		return fmt.Errorf("failed to create assembled file: %w", err)
	}
	defer out.Close()

	for _, c := range chunks {
		f, err := os.Open(c.Path)
		if err != nil {
			return fmt.Errorf("failed to open chunk %d: %w", c.Index, err)
		}
		if _, err := io.Copy(out, f); err != nil {
			f.Close()
			return fmt.Errorf("failed to copy chunk %d: %w", c.Index, err)
		}
		f.Close()
	}

	return nil
}

// downloadSimple performs a standard single-stream download.
func (d *ParallelDownloader) downloadSimple(ctx context.Context, url, destFile string) error {
	if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	d.setAuth(req)

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s failed: %s", url, resp.Status)
	}

	tmpFile := destFile + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return err
	}
	f.Close()

	return os.Rename(tmpFile, destFile)
}

func (d *ParallelDownloader) setAuth(req *http.Request) {
	if d.Token != "" {
		req.Header.Set("Authorization", "Bearer "+d.Token)
	}
}

// countingReader wraps an io.Reader and increments an atomic counter.
type countingReader struct {
	r       io.Reader
	counter *atomic.Int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	if n > 0 {
		cr.counter.Add(int64(n))
	}
	return n, err
}

// ComputeSHA256Stream computes SHA256 of a reader without loading the full
// contents into memory.
func ComputeSHA256Stream(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
