package metasearch

import (
	"fmt"
	"testing"
)

func TestSearch(t *testing.T) {
	results := Search("golang concurrency")
	fmt.Printf("Total results: %d\n", len(results))
	for i, r := range results {
		fmt.Printf("[%d] [%s] %s -> %s\n", i, r.Source, r.Title, r.URL)
	}
	if len(results) == 0 {
		t.Fatal("Expected results, got 0")
	}
}

func TestDDGOnly(t *testing.T) {
	results := SearchDuckDuckGo("golang concurrency")
	fmt.Printf("DDG results: %d\n", len(results))
	for i, r := range results {
		fmt.Printf("[%d] %s -> %s\n", i, r.Title, r.URL)
	}
}

func TestWikiOnly(t *testing.T) {
	results := SearchWikipedia("golang")
	fmt.Printf("Wiki results: %d\n", len(results))
	for i, r := range results {
		fmt.Printf("[%d] %s -> %s\n", i, r.Title, r.URL)
	}
}
