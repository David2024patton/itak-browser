package metasearch

import (
	"sort"
	"strings"
	"sync"
)

// SearchResult represents a single aggregated hit
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
	Source  string // e.g. "DuckDuckGo", "Wikipedia"
}

// Search runs a concurrent meta-search across all registered providers
func Search(query string) []SearchResult {
	var wg sync.WaitGroup
	resultsCh := make(chan []SearchResult, 10)

	// Registered providers
	providers := []func(string) []SearchResult{
		SearchDuckDuckGo,
		SearchWikipedia,
	}

	for _, p := range providers {
		wg.Add(1)
		go func(provider func(string) []SearchResult) {
			defer wg.Done()
			// Each provider handles its own HTTP timeouts
			res := provider(query)
			if len(res) > 0 {
				resultsCh <- res
			}
		}(p)
	}

	// Wait and close channel in background
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var allResults []SearchResult
	for resList := range resultsCh {
		allResults = append(allResults, resList...)
	}

	return deduplicateAndSort(allResults)
}

func deduplicateAndSort(results []SearchResult) []SearchResult {
	seen := make(map[string]bool)
	var unique []SearchResult

	for _, r := range results {
		// Normalize URLs for deduplication
		norm := strings.TrimRight(strings.ToLower(r.URL), "/")
		if !seen[norm] && r.Title != "" && r.URL != "" {
			seen[norm] = true
			unique = append(unique, r)
		}
	}

	// Sort by Wikipedia first (usually exact definitions), then others
	sort.Slice(unique, func(i, j int) bool {
		if unique[i].Source == "Wikipedia" && unique[j].Source != "Wikipedia" {
			return true
		}
		if unique[j].Source == "Wikipedia" && unique[i].Source != "Wikipedia" {
			return false
		}
		return i < j // preserve order of insertion otherwise
	})

	return unique
}
