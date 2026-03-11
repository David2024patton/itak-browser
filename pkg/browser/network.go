// Package browser - Chrome DevTools network request monitoring.
//
// What: Captures network requests and responses from the browser page.
// Why:  Agents diagnosing page load failures, API call results, or redirect
//       chains need visibility into the network layer.
// How:  Enables CDP Network domain, listens on RequestWillBeSent and
//       ResponseReceived events, correlates by requestId, stores in ring buffer.
package browser

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// NetworkEntry is a single captured request/response pair.
type NetworkEntry struct {
	RequestID  string    `json:"request_id"`
	URL        string    `json:"url"`
	Method     string    `json:"method"`
	Status     int64     `json:"status,omitempty"`      // 0 until response received
	StatusText string    `json:"status_text,omitempty"`
	MimeType   string    `json:"mime_type,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	Duration   string    `json:"duration,omitempty"` // Time from request to response
	Error      string    `json:"error,omitempty"`
}

// NetworkCapture manages a ring buffer of browser network requests.
type NetworkCapture struct {
	mu       sync.Mutex
	entries  []NetworkEntry
	pending  map[string]time.Time // requestId -> start time
	maxSize  int
}

// NewNetworkCapture creates a network capture buffer.
func NewNetworkCapture(maxSize int) *NetworkCapture {
	if maxSize <= 0 {
		maxSize = 512
	}
	return &NetworkCapture{
		entries: make([]NetworkEntry, 0, maxSize),
		pending: make(map[string]time.Time),
		maxSize: maxSize,
	}
}

// AddRequest records a new outgoing request.
func (nc *NetworkCapture) AddRequest(entry NetworkEntry) {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	nc.pending[entry.RequestID] = entry.Timestamp

	if len(nc.entries) >= nc.maxSize {
		nc.entries = nc.entries[1:]
	}
	nc.entries = append(nc.entries, entry)
}

// UpdateResponse updates a request entry with its response data.
func (nc *NetworkCapture) UpdateResponse(requestID string, status int64, statusText, mimeType string) {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	for i := len(nc.entries) - 1; i >= 0; i-- {
		if nc.entries[i].RequestID == requestID {
			nc.entries[i].Status = status
			nc.entries[i].StatusText = statusText
			nc.entries[i].MimeType = mimeType
			if start, ok := nc.pending[requestID]; ok {
				nc.entries[i].Duration = time.Since(start).Truncate(time.Millisecond).String()
				delete(nc.pending, requestID)
			}
			return
		}
	}
}

// Entries returns all captured network entries (oldest first).
func (nc *NetworkCapture) Entries() []NetworkEntry {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	result := make([]NetworkEntry, len(nc.entries))
	copy(result, nc.entries)
	return result
}

// Clear empties the network buffer.
func (nc *NetworkCapture) Clear() {
	nc.mu.Lock()
	defer nc.mu.Unlock()
	nc.entries = nc.entries[:0]
	nc.pending = make(map[string]time.Time)
}

// EnableNetworkCapture wires up CDP event listeners for network monitoring.
// Must be called after the browser context is created.
func EnableNetworkCapture(ctx context.Context, capture *NetworkCapture) error {
	// Enable the Network domain.
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.Enable().Do(ctx)
		}),
	); err != nil {
		return fmt.Errorf("network: enable: %w", err)
	}

	// Listen for network events.
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			capture.AddRequest(NetworkEntry{
				RequestID: string(e.RequestID),
				URL:       e.Request.URL,
				Method:    e.Request.Method,
				Timestamp: time.Now(),
			})

		case *network.EventResponseReceived:
			capture.UpdateResponse(
				string(e.RequestID),
				e.Response.Status,
				e.Response.StatusText,
				e.Response.MimeType,
			)
		}
	})

	return nil
}
