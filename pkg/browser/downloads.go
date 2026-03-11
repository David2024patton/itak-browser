// Package browser - File download tracking for AI agents.
//
// What: Track file downloads triggered by the browser.
// Why:  Agents clicking "Download PDF" have no way to confirm the download
//       completed or find the downloaded file. This provides that feedback.
// How:  Uses CDP Browser.setDownloadBehavior to set a download directory,
//       and monitors the download directory for new files.
package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
)

// DownloadEntry records a file download event.
type DownloadEntry struct {
	Filename    string `json:"filename"`
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	DiscoveredAt string `json:"discovered_at"`
}

// DownloadTracker monitors and records file downloads.
type DownloadTracker struct {
	mu          sync.RWMutex
	downloads   []DownloadEntry
	downloadDir string
	known       map[string]bool
}

// NewDownloadTracker creates a tracker with the given download directory.
func NewDownloadTracker(downloadDir string) *DownloadTracker {
	os.MkdirAll(downloadDir, 0755)
	return &DownloadTracker{
		downloads:   make([]DownloadEntry, 0),
		downloadDir: downloadDir,
		known:       make(map[string]bool),
	}
}

// Entries returns all tracked downloads (thread-safe copy).
func (dt *DownloadTracker) Entries() []DownloadEntry {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	result := make([]DownloadEntry, len(dt.downloads))
	copy(result, dt.downloads)
	return result
}

// Clear removes all tracked downloads.
func (dt *DownloadTracker) Clear() {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.downloads = dt.downloads[:0]
}

// DownloadDir returns the configured download directory.
func (dt *DownloadTracker) DownloadDir() string {
	return dt.downloadDir
}

// Scan checks the download directory for new files since last scan.
func (dt *DownloadTracker) Scan() []DownloadEntry {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	entries, err := os.ReadDir(dt.downloadDir)
	if err != nil {
		return nil
	}

	var newFiles []DownloadEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fullPath := filepath.Join(dt.downloadDir, e.Name())
		if dt.known[fullPath] {
			continue
		}
		dt.known[fullPath] = true

		info, _ := e.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}

		entry := DownloadEntry{
			Filename:     e.Name(),
			Path:         fullPath,
			SizeBytes:    size,
			DiscoveredAt: time.Now().UTC().Format(time.RFC3339),
		}
		dt.downloads = append(dt.downloads, entry)
		newFiles = append(newFiles, entry)
	}
	return newFiles
}

// EnableDownloadTracking sets the Chrome download directory via CDP.
func EnableDownloadTracking(ctx context.Context, tracker *DownloadTracker) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorAllowAndName).
				WithDownloadPath(tracker.downloadDir).
				WithEventsEnabled(true).
				Do(ctx)
		}),
	)
}

// WaitForDownload polls the download directory until a new file appears or timeout.
func (dt *DownloadTracker) WaitForDownload(timeout time.Duration) (*DownloadEntry, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return nil, fmt.Errorf("download: timeout after %s", timeout)
		case <-ticker.C:
			newFiles := dt.Scan()
			if len(newFiles) > 0 {
				return &newFiles[len(newFiles)-1], nil
			}
		}
	}
}
