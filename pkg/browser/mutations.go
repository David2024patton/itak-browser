// Package browser - DOM mutation observation for AI agents.
//
// What: Watch for DOM changes and report them to the agent.
// Why:  Modern SPAs load content dynamically via AJAX, lazy-loading, and
//       websockets. Without mutation observation, agents must poll with
//       repeated snapshots to detect new content. This is wasteful and slow.
// How:  Injects a MutationObserver via JS that captures added/removed nodes
//       and attribute changes, storing them in a JS array that we read back.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// MutationEntry records a single DOM mutation.
type MutationEntry struct {
	Type      string `json:"type"`       // childList, attributes, characterData
	Target    string `json:"target"`     // CSS selector of the mutated element
	Added     int    `json:"added"`      // Number of added nodes
	Removed   int    `json:"removed"`    // Number of removed nodes
	Attribute string `json:"attribute,omitempty"` // Changed attribute name
	OldValue  string `json:"old_value,omitempty"`
}

// StartMutationObserver injects a MutationObserver that captures DOM changes.
// Call GetMutations() to retrieve the captured mutations.
func StartMutationObserver(ctx context.Context, selector string) error {
	if selector == "" {
		selector = "document.body"
	} else {
		selector = fmt.Sprintf(`document.querySelector(%q)`, selector)
	}

	js := fmt.Sprintf(`
		(function() {
			// Clean up any existing observer.
			if (window.__itakMutationObserver) {
				window.__itakMutationObserver.disconnect();
			}
			window.__itakMutations = [];

			const target = %s;
			if (!target) return false;

			function getSelector(el) {
				if (!el || !el.tagName) return '';
				if (el.id) return '#' + el.id;
				let path = el.tagName.toLowerCase();
				if (el.className && typeof el.className === 'string') {
					path += '.' + el.className.trim().split(/\s+/).join('.');
				}
				return path;
			}

			window.__itakMutationObserver = new MutationObserver(function(mutations) {
				for (const m of mutations) {
					if (window.__itakMutations.length >= 500) {
						window.__itakMutations.shift();
					}
					window.__itakMutations.push({
						type: m.type,
						target: getSelector(m.target),
						added: m.addedNodes ? m.addedNodes.length : 0,
						removed: m.removedNodes ? m.removedNodes.length : 0,
						attribute: m.attributeName || '',
						old_value: m.oldValue || ''
					});
				}
			});

			window.__itakMutationObserver.observe(target, {
				childList: true,
				attributes: true,
				characterData: true,
				subtree: true,
				attributeOldValue: true
			});

			return true;
		})()
	`, selector)

	var started bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &started)); err != nil {
		return fmt.Errorf("start mutation observer: %w", err)
	}
	if !started {
		return fmt.Errorf("mutation observer: target element not found")
	}
	return nil
}

// GetMutations retrieves captured DOM mutations since the observer started
// (or since the last call to GetMutations with clear=true).
func GetMutations(ctx context.Context, clear bool) ([]MutationEntry, error) {
	clearJS := ""
	if clear {
		clearJS = "window.__itakMutations = [];"
	}

	js := fmt.Sprintf(`
		(function() {
			const mutations = window.__itakMutations || [];
			%s
			return mutations;
		})()
	`, clearJS)

	var entries []MutationEntry
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &entries)); err != nil {
		return nil, fmt.Errorf("get mutations: %w", err)
	}
	return entries, nil
}

// StopMutationObserver disconnects the observer.
func StopMutationObserver(ctx context.Context) error {
	js := `
		(function() {
			if (window.__itakMutationObserver) {
				window.__itakMutationObserver.disconnect();
				window.__itakMutationObserver = null;
			}
			window.__itakMutations = [];
			return true;
		})()
	`
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}
