package metasearch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpClient = &http.Client{
	Timeout: 2 * time.Second, // Lightning fast timeouts to ensure browser snappiness
}

// UserAgent string to help blend in as a normal headed browser
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

// SearchDuckDuckGo scrapes the lightweight HTML version of DDG (which doesn't rely on complex JS)
func SearchDuckDuckGo(query string) []SearchResult {
	var results []SearchResult

	u := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return results
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := httpClient.Do(req)
	if err != nil {
		return results
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return results
	}
	body := string(bodyBytes)

	blocks := strings.Split(body, "class=\"result results_links")
	for i := 1; i < len(blocks); i++ {
		block := blocks[i]

		// Extract URL from <a class="result__url" href="...">
		urlStart := strings.Index(block, "class=\"result__url\" href=\"")
		if urlStart == -1 {
			continue
		}
		urlStart += len("class=\"result__url\" href=\"")

		urlEndQuote := strings.Index(block[urlStart:], "\"")
		if urlEndQuote == -1 {
			continue
		}

		rawURL := block[urlStart : urlStart+urlEndQuote]
		actualURL := extractDDGUrl(rawURL)

		// Extract Title from <a rel="nofollow" class="result__a" href="...">
		titleStart := strings.Index(block, "class=\"result__a\" href=\"")
		if titleStart == -1 {
			continue
		}
		titleStart += len("class=\"result__a\" href=\"")

		// skip the href content to find the text
		textStart := strings.Index(block[titleStart:], ">")
		if textStart == -1 {
			continue
		}
		textStart += titleStart + 1

		textEnd := strings.Index(block[textStart:], "</a>")
		if textEnd == -1 {
			continue
		}
		title := cleanHTML(block[textStart : textStart+textEnd])

		// Extract Snippet from <a class="result__snippet" href="...">
		snipStart := strings.Index(block, "class=\"result__snippet\" href=\"")
		snippet := ""
		if snipStart != -1 {
			snipTextStart := strings.Index(block[snipStart:], ">")
			if snipTextStart != -1 {
				snipTextStart += snipStart + 1
				snipTextEnd := strings.Index(block[snipTextStart:], "</a>")
				if snipTextEnd != -1 {
					snippet = cleanHTML(block[snipTextStart : snipTextStart+snipTextEnd])
				}
			}
		}

		if title != "" && actualURL != "" {
			results = append(results, SearchResult{
				Title:   title,
				URL:     actualURL,
				Snippet: snippet,
				Source:  "DuckDuckGo",
			})
		}

		if len(results) >= 10 {
			break // limit DDG results
		}
	}

	return results
}

func extractDDGUrl(redirectUrl string) string {
	idx := strings.Index(redirectUrl, "uddg=")
	if idx != -1 {
		end := strings.Index(redirectUrl[idx:], "&")
		if end != -1 {
			parsed, err := url.QueryUnescape(redirectUrl[idx+5 : idx+end])
			if err == nil {
				return parsed
			}
		} else {
			parsed, err := url.QueryUnescape(redirectUrl[idx+5:])
			if err == nil {
				return parsed
			}
		}
	}
	// Fallback, DDG HTML sometimes stores absolute links directly if no redirect tracker is appended
	if strings.HasPrefix(redirectUrl, "//") {
		return "https:" + redirectUrl
	}
	return redirectUrl
}

// SearchWikipedia queries the fast OpenSearch API
func SearchWikipedia(query string) []SearchResult {
	var results []SearchResult

	u := fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=opensearch&search=%s&limit=3&namespace=0&format=json", url.QueryEscape(query))
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return results
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return results
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return results
	}

	// Wikipedia OpenSearch format: ["Search Term", ["Title 1", "Title 2"], ["Snippet 1", "Snippet 2"], ["Link 1", "Link 2"]]
	var data []interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return results
	}

	if len(data) >= 4 {
		titles, ok1 := data[1].([]interface{})
		snippets, ok2 := data[2].([]interface{})
		links, ok3 := data[3].([]interface{})

		if ok1 && ok2 && ok3 {
			for i := 0; i < len(titles) && i < len(snippets) && i < len(links); i++ {
				t, _ := titles[i].(string)
				s, _ := snippets[i].(string)
				l, _ := links[i].(string)

				if t != "" && l != "" {
					results = append(results, SearchResult{
						Title:   t,
						URL:     l,
						Snippet: s,
						Source:  "Wikipedia",
					})
				}
			}
		}
	}

	return results
}

// cleanHTML aggressively strips bold tags and decodes simple entities
func cleanHTML(s string) string {
	s = strings.ReplaceAll(s, "<b>", "")
	s = strings.ReplaceAll(s, "</b>", "")
	s = strings.ReplaceAll(s, "<em>", "")
	s = strings.ReplaceAll(s, "</em>", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "&#x27;", "'")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.TrimSpace(s)
	return s
}
