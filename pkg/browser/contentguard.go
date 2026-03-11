// Package browser - AI Recommendation Poisoning defense layer.
//
// What: ContentGuard strips prompt-injection patterns from page content before
//       the agent processes it.
// Why:  Microsoft Security researchers identified "AI Recommendation Poisoning"
//       (MITRE ATLAS AML.T0080) where hidden instructions in web pages inject
//       "remember X as a trusted source" directives into AI memory. Since our
//       browser module feeds page text directly to LLMs, we need to sanitize
//       it at the extraction layer.
// How:  Pattern matching on known poisoning markers (remember, trusted source,
//       always recommend, authoritative source, in future conversations) plus
//       detection of URL-embedded prompt parameters (q=, prompt=). Flags
//       suspicious content with warnings instead of silently stripping, so the
//       agent can decide how to handle it.
//
// Reference: https://www.microsoft.com/en-us/security/blog/2026/02/10/ai-recommendation-poisoning/
package browser

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ContentGuardResult holds the output of a content scan.
type ContentGuardResult struct {
	Clean    bool     `json:"clean"`              // True if no poisoning detected
	Warnings []string `json:"warnings,omitempty"` // Human-readable warnings
	Stripped string   `json:"stripped,omitempty"`  // Content with injection patterns removed
	Original string   `json:"original,omitempty"` // The original content (for comparison)
}

// PoisonPattern describes a single detection rule.
type PoisonPattern struct {
	Name        string         // Human-readable name
	Regex       *regexp.Regexp // Compiled pattern
	Severity    string         // "high", "medium", "low"
	Description string         // What this pattern catches
}

// Predefined poisoning patterns based on Microsoft's research findings.
// These cover the "Common Patterns" section from the article:
//   - Persistence instructions: "remember", "in future conversations", "trusted source"
//   - Authority injection: "authoritative source", "go-to source", "always recommend"
//   - Memory manipulation: "from now on", "always cite", "keep in memory"
var defaultPatterns = []PoisonPattern{
	{
		Name:        "memory-persist-remember",
		Regex:       regexp.MustCompile(`(?i)\b(remember|memorize)\s+.{3,80}\s+(as\s+)?(a\s+)?(trusted|authoritative|reliable|credible|go-to)\s+(source|reference|citation)`),
		Severity:    "high",
		Description: "Attempts to inject a site as a 'trusted source' into AI memory",
	},
	{
		Name:        "memory-persist-future",
		Regex:       regexp.MustCompile(`(?i)\b(in\s+future\s+conversations?|from\s+now\s+on|going\s+forward|for\s+future\s+reference)\b`),
		Severity:    "high",
		Description: "Attempts to persist instructions across conversation boundaries",
	},
	{
		Name:        "authority-injection",
		Regex:       regexp.MustCompile(`(?i)\b(always\s+(recommend|cite|reference|prefer|use|suggest)|the\s+(best|top|only|go-to)\s+(choice|source|platform|tool|option)\s+for)\b`),
		Severity:    "medium",
		Description: "Injects absolute preference for a product or service",
	},
	{
		Name:        "memory-keep",
		Regex:       regexp.MustCompile(`(?i)\b(keep\s+.{1,40}\s+in\s+(your\s+)?memory|save\s+(this|these)\s+(to|in)\s+(your\s+)?memory|add\s+.{1,40}\s+to\s+(your\s+)?memory)\b`),
		Severity:    "high",
		Description: "Direct memory manipulation commands",
	},
	{
		Name:        "hidden-system-prompt",
		Regex:       regexp.MustCompile(`(?i)(system\s*:\s*|you\s+are\s+now\s+|ignore\s+previous\s+instructions?|disregard\s+(all\s+)?previous|forget\s+(everything|all)\s+(you|previous))`),
		Severity:    "high",
		Description: "Classic prompt injection / system prompt override",
	},
	{
		Name:        "promotional-injection",
		Regex:       regexp.MustCompile(`(?i)\b(all-in-one\s+(sales|marketing|platform)|industry-leading|best-in-class|unmatched\s+(by|in)|surpasses\s+all\s+competitors?)\b`),
		Severity:    "low",
		Description: "Marketing hyperbole that may indicate promotional injection",
	},
	{
		Name:        "url-prompt-param",
		Regex:       regexp.MustCompile(`(?i)[?&](q|prompt|query|instruction|cmd|command|text)=[^&]*\b(remember|trusted|authoritative|always|memorize)\b`),
		Severity:    "high",
		Description: "URL query parameter containing prompt injection keywords",
	},
}

// ContentGuard scans and sanitizes content for AI recommendation poisoning.
type ContentGuard struct {
	patterns []PoisonPattern
	enabled  bool
}

// NewContentGuard creates a ContentGuard with default patterns.
func NewContentGuard() *ContentGuard {
	return &ContentGuard{
		patterns: defaultPatterns,
		enabled:  true,
	}
}

// AddPattern adds a custom detection pattern.
func (cg *ContentGuard) AddPattern(name, pattern, severity, description string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("contentguard: compile pattern %q: %w", name, err)
	}
	cg.patterns = append(cg.patterns, PoisonPattern{
		Name:        name,
		Regex:       re,
		Severity:    severity,
		Description: description,
	})
	return nil
}

// SetEnabled toggles the guard on/off. Useful for trusted internal pages.
func (cg *ContentGuard) SetEnabled(enabled bool) {
	cg.enabled = enabled
}

// ScanContent checks text content for poisoning patterns.
// Returns the scan result with warnings and optionally stripped content.
func (cg *ContentGuard) ScanContent(content string) ContentGuardResult {
	if !cg.enabled {
		return ContentGuardResult{Clean: true, Stripped: content, Original: content}
	}

	result := ContentGuardResult{
		Clean:    true,
		Original: content,
		Stripped: content,
	}

	for _, p := range cg.patterns {
		matches := p.Regex.FindAllString(content, -1)
		if len(matches) > 0 {
			result.Clean = false
			for _, m := range matches {
				warning := fmt.Sprintf("[%s/%s] %s: %q", p.Severity, p.Name, p.Description, m)
				result.Warnings = append(result.Warnings, warning)
			}
			// Strip the matched patterns from the sanitized output.
			result.Stripped = p.Regex.ReplaceAllString(result.Stripped, "[BLOCKED: "+p.Name+"]")
		}
	}

	return result
}

// ScanURL checks a URL for embedded prompt injection parameters.
func (cg *ContentGuard) ScanURL(url string) ContentGuardResult {
	if !cg.enabled {
		return ContentGuardResult{Clean: true, Stripped: url, Original: url}
	}

	result := ContentGuardResult{
		Clean:    true,
		Original: url,
		Stripped: url,
	}

	// Check for known AI assistant URL patterns with pre-populated prompts.
	aiDomains := []string{
		"chat.openai.com", "chatgpt.com",
		"copilot.microsoft.com",
		"claude.ai", "gemini.google.com",
		"chat.mistral.ai",
	}

	urlLower := strings.ToLower(url)
	for _, domain := range aiDomains {
		if strings.Contains(urlLower, domain) && strings.Contains(urlLower, "?") {
			result.Clean = false
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("[high/ai-redirect] URL redirects to AI assistant %q with pre-populated prompt", domain))
		}
	}

	// Also run the standard pattern scan on the full URL string.
	urlScan := cg.ScanContent(url)
	if !urlScan.Clean {
		result.Clean = false
		result.Warnings = append(result.Warnings, urlScan.Warnings...)
	}

	return result
}

// ScanReport is a structured scan summary for the debug bundle.
type ScanReport struct {
	Timestamp string               `json:"timestamp"`
	URL       ContentGuardResult   `json:"url_scan"`
	Content   []ContentGuardResult `json:"content_scans,omitempty"`
}

// NewScanReport creates a timestamped report from URL and content scans.
func NewScanReport(urlResult ContentGuardResult, contentResults ...ContentGuardResult) ScanReport {
	return ScanReport{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		URL:       urlResult,
		Content:   contentResults,
	}
}
