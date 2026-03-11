// Package browser - CDP request blocking and domain filtering.
//
// What: Blocks network requests to ads, trackers, and known malicious domains.
// Why:  Agents waste tokens parsing ad/tracker content from snapshots.
//       Blocking at the CDP level prevents these resources from loading
//       entirely, making pages faster and cleaner.
// How:  Uses CDP Fetch.enable to intercept requests and fail those matching
//       blocked domain patterns.
package browser

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// RequestBlocker manages domain-level request blocking via CDP.
type RequestBlocker struct {
	mu             sync.RWMutex
	blockedDomains map[string]bool
	enabled        bool
	stats          BlockStats
}

// BlockStats tracks how many requests have been blocked.
type BlockStats struct {
	TotalBlocked int            `json:"total_blocked"`
	ByDomain     map[string]int `json:"by_domain"`
}

// NewRequestBlocker creates a blocker with default ad/tracker domains.
func NewRequestBlocker() *RequestBlocker {
	rb := &RequestBlocker{
		blockedDomains: make(map[string]bool),
		enabled:        true,
		stats:          BlockStats{ByDomain: make(map[string]int)},
	}

	// Default blocked domains: common ad networks and trackers.
	defaults := []string{
		// Ad networks
		"doubleclick.net", "googlesyndication.com", "googleadservices.com",
		"adservice.google.com", "pagead2.googlesyndication.com",
		"ads.facebook.com", "pixel.facebook.com",
		"ads.twitter.com", "ads.linkedin.com",
		"amazon-adsystem.com", "media.net",
		// Trackers
		"google-analytics.com", "googletagmanager.com",
		"hotjar.com", "fullstory.com", "mouseflow.com",
		"segment.io", "segment.com", "mixpanel.com",
		"amplitude.com", "heap.io", "heapanalytics.com",
		// Social widgets that load heavy iframes
		"platform.twitter.com", "connect.facebook.net",
	}

	for _, d := range defaults {
		rb.blockedDomains[d] = true
	}

	return rb
}

// AddDomain adds a domain to the block list.
func (rb *RequestBlocker) AddDomain(domain string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.blockedDomains[strings.ToLower(domain)] = true
}

// RemoveDomain removes a domain from the block list.
func (rb *RequestBlocker) RemoveDomain(domain string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	delete(rb.blockedDomains, strings.ToLower(domain))
}

// SetEnabled toggles blocking on/off.
func (rb *RequestBlocker) SetEnabled(enabled bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.enabled = enabled
}

// IsBlocked checks if a URL should be blocked.
func (rb *RequestBlocker) IsBlocked(url string) (bool, string) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if !rb.enabled {
		return false, ""
	}

	urlLower := strings.ToLower(url)
	for domain := range rb.blockedDomains {
		if strings.Contains(urlLower, domain) {
			return true, domain
		}
	}
	return false, ""
}

// RecordBlock increments the block counter for a domain.
func (rb *RequestBlocker) RecordBlock(domain string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.stats.TotalBlocked++
	rb.stats.ByDomain[domain]++
}

// Stats returns current blocking statistics.
func (rb *RequestBlocker) Stats() BlockStats {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	result := BlockStats{
		TotalBlocked: rb.stats.TotalBlocked,
		ByDomain:     make(map[string]int, len(rb.stats.ByDomain)),
	}
	for k, v := range rb.stats.ByDomain {
		result.ByDomain[k] = v
	}
	return result
}

// EnableRequestBlocking wires up CDP Fetch interception.
// Must be called after the browser context is created.
func EnableRequestBlocking(ctx context.Context, blocker *RequestBlocker) error {
	// Enable Fetch domain to intercept requests.
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable fetch interception for all request patterns.
			return fetch.Enable().
				WithPatterns([]*fetch.RequestPattern{
					{URLPattern: "*", RequestStage: fetch.RequestStageRequest},
				}).Do(ctx)
		}),
	); err != nil {
		return fmt.Errorf("blocker: enable fetch: %w", err)
	}

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventRequestPaused:
			go func() {
				url := e.Request.URL
				blocked, domain := blocker.IsBlocked(url)

				if blocked {
					blocker.RecordBlock(domain)
					// Fail the request.
					chromedp.Run(ctx,
						chromedp.ActionFunc(func(ctx context.Context) error {
							return fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient).Do(ctx)
						}),
					)
				} else {
					// Continue the request.
					chromedp.Run(ctx,
						chromedp.ActionFunc(func(ctx context.Context) error {
							return fetch.ContinueRequest(e.RequestID).Do(ctx)
						}),
					)
				}
			}()
		}
	})

	return nil
}
