// Package browser - ThreatLedger for public poisoning site tracking.
//
// What: Persistent log of every website caught with AI recommendation poisoning.
// Why:  When the browser detects injection prompts in page content, we record
//       the offending URL, domain, matched patterns, and severity. This data
//       is exposed via the daemon API so an external website can poll it and
//       display a public "wall of shame" or threat feed.
// How:  Thread-safe append-only ledger backed by a JSON file on disk.
//       New entries are written immediately so no data is lost on crash.
//       The daemon exposes GET /threats for website polling.
//
// Reference: Microsoft AI Recommendation Poisoning research
// https://www.microsoft.com/en-us/security/blog/2026/02/10/ai-recommendation-poisoning/
package browser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ThreatEntry records a single poisoning detection event.
type ThreatEntry struct {
	ID        string   `json:"id"`                    // Unique entry ID
	URL       string   `json:"url"`                   // Full URL of the offending page
	Domain    string   `json:"domain"`                // Extracted domain for grouping
	Timestamp string   `json:"timestamp"`             // ISO 8601 detection time
	SessionID string   `json:"session_id"`            // Which browser session found it
	Severity  string   `json:"severity"`              // Highest severity among matched patterns
	Patterns  []string `json:"patterns"`              // Names of matched poisoning patterns
	Warnings  []string `json:"warnings"`              // Full warning messages
	TreeSnippet string `json:"tree_snippet,omitempty"` // First 500 chars of the snapshot tree for context
}

// ThreatLedger is a persistent, append-only log of poisoning detections.
type ThreatLedger struct {
	mu         sync.RWMutex
	entries    []ThreatEntry
	filePath   string
	counter    int64
	webhookURL string        // If set, POST each detection here in real-time
}

// NewThreatLedger creates or loads a ledger from the given directory.
func NewThreatLedger(dir string) (*ThreatLedger, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("threatledger: create dir: %w", err)
	}

	fp := filepath.Join(dir, "threat_ledger.json")
	tl := &ThreatLedger{
		filePath: fp,
		entries:  make([]ThreatEntry, 0),
	}

	// Load existing entries if the file exists.
	data, err := os.ReadFile(fp)
	if err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &tl.entries); err != nil {
			// Corrupted file; start fresh but keep a backup.
			backup := fp + ".bak"
			os.WriteFile(backup, data, 0644)
			tl.entries = make([]ThreatEntry, 0)
		}
		tl.counter = int64(len(tl.entries))
	}

	return tl, nil
}

// SetWebhook configures a URL to receive real-time POST notifications
// whenever a new poisoning detection is recorded.
func (tl *ThreatLedger) SetWebhook(url string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.webhookURL = url
}

// Record adds a new threat entry and persists to disk.
// Called by ContentGuard when poisoning is detected during browsing.
func (tl *ThreatLedger) Record(pageURL, sessionID string, result ContentGuardResult, treeSnippet string) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	tl.counter++

	// Extract domain from URL.
	domain := pageURL
	if parsed, err := url.Parse(pageURL); err == nil && parsed.Host != "" {
		domain = parsed.Host
	}

	// Determine highest severity from warnings.
	severity := "low"
	patterns := make([]string, 0)
	for _, w := range result.Warnings {
		if len(w) > 6 {
			// Parse severity from "[severity/name] ..." format.
			if w[0] == '[' {
				for i := 1; i < len(w); i++ {
					if w[i] == '/' {
						sev := w[1:i]
						if sev == "high" {
							severity = "high"
						} else if sev == "medium" && severity != "high" {
							severity = "medium"
						}
						// Extract pattern name.
						for j := i + 1; j < len(w); j++ {
							if w[j] == ']' {
								patterns = append(patterns, w[i+1:j])
								break
							}
						}
						break
					}
				}
			}
		}
	}

	// Truncate tree snippet.
	snippet := treeSnippet
	if len(snippet) > 500 {
		snippet = snippet[:500] + "..."
	}

	entry := ThreatEntry{
		ID:          fmt.Sprintf("threat_%d_%d", time.Now().Unix(), tl.counter),
		URL:         pageURL,
		Domain:      domain,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		SessionID:   sessionID,
		Severity:    severity,
		Patterns:    patterns,
		Warnings:    result.Warnings,
		TreeSnippet: snippet,
	}

	tl.entries = append(tl.entries, entry)

	// Fire webhook asynchronously if configured.
	if tl.webhookURL != "" {
		go tl.pushWebhook(entry)
	}

	return tl.persist()
}

// pushWebhook sends a detection entry to the configured webhook URL.
// Retries up to 3 times with 1-second backoff.
func (tl *ThreatLedger) pushWebhook(entry ThreatEntry) {
	tl.mu.RLock()
	webhookURL := tl.webhookURL
	tl.mu.RUnlock()

	if webhookURL == "" {
		return
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(payload))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return // Success.
			}
		}
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
}

// Entries returns all recorded threat entries (thread-safe copy).
func (tl *ThreatLedger) Entries() []ThreatEntry {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	result := make([]ThreatEntry, len(tl.entries))
	copy(result, tl.entries)
	return result
}

// Stats returns aggregate statistics for the threat feed.
type ThreatStats struct {
	TotalDetections int            `json:"total_detections"`
	UniqueDomains   int            `json:"unique_domains"`
	BySeverity      map[string]int `json:"by_severity"`
	TopDomains      []DomainCount  `json:"top_domains"`
	Since           string         `json:"since"`
}

// DomainCount pairs a domain with its detection count.
type DomainCount struct {
	Domain string `json:"domain"`
	Count  int    `json:"count"`
}

// Stats computes aggregate threat statistics.
func (tl *ThreatLedger) Stats() ThreatStats {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	stats := ThreatStats{
		TotalDetections: len(tl.entries),
		BySeverity:      make(map[string]int),
	}

	domainCounts := make(map[string]int)
	var earliest time.Time

	for _, e := range tl.entries {
		stats.BySeverity[e.Severity]++
		domainCounts[e.Domain]++
		if t, err := time.Parse(time.RFC3339, e.Timestamp); err == nil {
			if earliest.IsZero() || t.Before(earliest) {
				earliest = t
			}
		}
	}

	stats.UniqueDomains = len(domainCounts)
	if !earliest.IsZero() {
		stats.Since = earliest.Format(time.RFC3339)
	}

	// Top domains (simple sort by count, max 20).
	for domain, count := range domainCounts {
		stats.TopDomains = append(stats.TopDomains, DomainCount{Domain: domain, Count: count})
	}
	// Sort descending by count (bubble sort is fine for small N).
	for i := 0; i < len(stats.TopDomains); i++ {
		for j := i + 1; j < len(stats.TopDomains); j++ {
			if stats.TopDomains[j].Count > stats.TopDomains[i].Count {
				stats.TopDomains[i], stats.TopDomains[j] = stats.TopDomains[j], stats.TopDomains[i]
			}
		}
	}
	if len(stats.TopDomains) > 20 {
		stats.TopDomains = stats.TopDomains[:20]
	}

	return stats
}

// persist writes the full ledger to disk.
func (tl *ThreatLedger) persist() error {
	data, err := json.MarshalIndent(tl.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("threatledger: marshal: %w", err)
	}
	return os.WriteFile(tl.filePath, data, 0644)
}
