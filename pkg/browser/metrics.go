// Package browser - Page performance metrics collection.
//
// What: Collects DOM size, load timing, and memory usage metrics.
// Why:  Agents need to know if a page is too heavy to snapshot, or if it
//       hasn't finished loading. Metrics help decide when to proceed.
// How:  Runs JS via chromedp to query Performance API, DOM counters,
//       and memory info.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// PageMetrics holds performance data about the current page.
type PageMetrics struct {
	DOMNodes     int     `json:"dom_nodes"`                // Total DOM node count
	DOMDepth     int     `json:"dom_depth"`                // Maximum DOM tree depth
	LoadTime     float64 `json:"load_time_ms"`             // Page load time in ms
	DOMReady     float64 `json:"dom_ready_ms"`             // DOMContentLoaded time in ms
	TransferSize float64 `json:"transfer_size_kb"`         // Total transfer size in KB
	ResourceCount int    `json:"resource_count"`           // Number of loaded resources
	JSHeapUsed   float64 `json:"js_heap_used_mb,omitempty"` // JS heap in MB (Chrome only)
	JSHeapTotal  float64 `json:"js_heap_total_mb,omitempty"`
}

// CollectPageMetrics gathers performance data from the current page.
func CollectPageMetrics(ctx context.Context) (PageMetrics, error) {
	var metrics PageMetrics

	js := `
		(function() {
			// DOM metrics.
			const allNodes = document.querySelectorAll('*');
			let maxDepth = 0;
			function getDepth(el) {
				let depth = 0;
				let node = el;
				while (node.parentNode) { depth++; node = node.parentNode; }
				return depth;
			}
			for (let i = 0; i < Math.min(allNodes.length, 5000); i++) {
				const d = getDepth(allNodes[i]);
				if (d > maxDepth) maxDepth = d;
			}

			// Performance timing.
			const perf = performance.getEntriesByType('navigation')[0] || {};
			const loadTime = perf.loadEventEnd ? (perf.loadEventEnd - perf.startTime) : 0;
			const domReady = perf.domContentLoadedEventEnd ? (perf.domContentLoadedEventEnd - perf.startTime) : 0;

			// Resource stats.
			const resources = performance.getEntriesByType('resource');
			let totalTransfer = 0;
			for (const r of resources) {
				totalTransfer += r.transferSize || 0;
			}

			// Memory (Chrome-specific).
			let heapUsed = 0, heapTotal = 0;
			if (performance.memory) {
				heapUsed = performance.memory.usedJSHeapSize / (1024 * 1024);
				heapTotal = performance.memory.totalJSHeapSize / (1024 * 1024);
			}

			return {
				dom_nodes: allNodes.length,
				dom_depth: maxDepth,
				load_time_ms: Math.round(loadTime * 100) / 100,
				dom_ready_ms: Math.round(domReady * 100) / 100,
				transfer_size_kb: Math.round(totalTransfer / 1024 * 100) / 100,
				resource_count: resources.length,
				js_heap_used_mb: Math.round(heapUsed * 100) / 100,
				js_heap_total_mb: Math.round(heapTotal * 100) / 100
			};
		})()
	`

	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &metrics)); err != nil {
		return PageMetrics{}, fmt.Errorf("metrics: %w", err)
	}

	return metrics, nil
}
