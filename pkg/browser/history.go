// Package browser - Navigation history tracking.
//
// What: Records every URL the agent visits within a session.
// Why:  The teacher model needs to show students the full journey:
//       "We went from the homepage to the login page to the dashboard."
//       Also useful for debugging agent navigation loops.
// How:  Simple append-only list updated by the Engine on every Open() call.
package browser

import (
	"sync"
	"time"
)

// HistoryEntry records a single navigation event.
type HistoryEntry struct {
	URL       string `json:"url"`
	Title     string `json:"title,omitempty"`
	Timestamp string `json:"timestamp"`
	Index     int    `json:"index"`
}

// NavigationHistory tracks URLs visited in a session.
type NavigationHistory struct {
	mu      sync.RWMutex
	entries []HistoryEntry
}

// NewNavigationHistory creates an empty history tracker.
func NewNavigationHistory() *NavigationHistory {
	return &NavigationHistory{
		entries: make([]HistoryEntry, 0),
	}
}

// Add records a new URL visit.
func (nh *NavigationHistory) Add(url, title string) {
	nh.mu.Lock()
	defer nh.mu.Unlock()
	nh.entries = append(nh.entries, HistoryEntry{
		URL:       url,
		Title:     title,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Index:     len(nh.entries),
	})
}

// Entries returns all navigation entries (thread-safe copy).
func (nh *NavigationHistory) Entries() []HistoryEntry {
	nh.mu.RLock()
	defer nh.mu.RUnlock()
	result := make([]HistoryEntry, len(nh.entries))
	copy(result, nh.entries)
	return result
}

// Current returns the most recent URL, or empty string if no history.
func (nh *NavigationHistory) Current() string {
	nh.mu.RLock()
	defer nh.mu.RUnlock()
	if len(nh.entries) == 0 {
		return ""
	}
	return nh.entries[len(nh.entries)-1].URL
}

// Len returns the number of entries.
func (nh *NavigationHistory) Len() int {
	nh.mu.RLock()
	defer nh.mu.RUnlock()
	return len(nh.entries)
}

// Clear removes all history entries.
func (nh *NavigationHistory) Clear() {
	nh.mu.Lock()
	defer nh.mu.Unlock()
	nh.entries = nh.entries[:0]
}
