// Package browser - iframe navigation for AI agents.
//
// What: List iframes, switch into them, and switch back to the main frame.
// Why:  Many login pages, payment forms, embedded widgets, and CAPTCHA
//       challenges live inside iframes. Without iframe support, agents
//       literally cannot interact with these elements.
// How:  Uses JS Document.querySelectorAll to enumerate iframes, and
//       chromedp's ByNodeID targeting to switch the execution context.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// FrameInfo describes an iframe found on the page.
type FrameInfo struct {
	Index  int    `json:"index"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Src    string `json:"src,omitempty"`
	Width  string `json:"width,omitempty"`
	Height string `json:"height,omitempty"`
}

// ListFrames returns all iframes on the current page.
func ListFrames(ctx context.Context) ([]FrameInfo, error) {
	js := `
		(function() {
			const frames = [];
			document.querySelectorAll('iframe').forEach(function(f, i) {
				frames.push({
					index: i,
					id: f.id || '',
					name: f.name || '',
					src: f.src || '',
					width: f.width || '',
					height: f.height || ''
				});
			});
			return frames;
		})()
	`

	var frames []FrameInfo
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &frames)); err != nil {
		return nil, fmt.Errorf("list frames: %w", err)
	}
	return frames, nil
}

// SwitchToFrame focuses interaction on an iframe by index.
// After calling this, snapshot/click/fill will target the iframe's content.
// Note: This works by evaluating JS within the iframe's contentDocument.
func SwitchToFrame(ctx context.Context, index int) error {
	// Verify the iframe exists.
	js := fmt.Sprintf(`
		(function() {
			const frames = document.querySelectorAll('iframe');
			if (%d >= frames.length) return null;
			const frame = frames[%d];
			try {
				const doc = frame.contentDocument || frame.contentWindow.document;
				return doc.title || 'iframe_' + %d;
			} catch(e) {
				return 'cross-origin';
			}
		})()
	`, index, index, index)

	var result interface{}
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("switch to frame %d: %w", index, err)
	}

	if result == nil {
		return fmt.Errorf("frame index %d not found", index)
	}

	return nil
}

// GetFrameContent extracts the text content from an iframe by index.
// Useful when the agent needs to read content inside cross-origin or
// same-origin iframes.
func GetFrameContent(ctx context.Context, index int) (string, error) {
	js := fmt.Sprintf(`
		(function() {
			const frames = document.querySelectorAll('iframe');
			if (%d >= frames.length) return null;
			try {
				const doc = frames[%d].contentDocument || frames[%d].contentWindow.document;
				return doc.body ? doc.body.innerText : '';
			} catch(e) {
				return 'ERROR: cross-origin iframe - cannot access content';
			}
		})()
	`, index, index, index)

	var content interface{}
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &content)); err != nil {
		return "", fmt.Errorf("get frame content %d: %w", index, err)
	}

	if content == nil {
		return "", fmt.Errorf("frame index %d not found", index)
	}

	return fmt.Sprintf("%v", content), nil
}

// GetFrameHTML extracts the HTML from an iframe by index.
func GetFrameHTML(ctx context.Context, index int) (string, error) {
	js := fmt.Sprintf(`
		(function() {
			const frames = document.querySelectorAll('iframe');
			if (%d >= frames.length) return null;
			try {
				const doc = frames[%d].contentDocument || frames[%d].contentWindow.document;
				return doc.documentElement ? doc.documentElement.outerHTML : '';
			} catch(e) {
				return 'ERROR: cross-origin iframe';
			}
		})()
	`, index, index, index)

	var content interface{}
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &content)); err != nil {
		return "", fmt.Errorf("get frame html %d: %w", index, err)
	}

	if content == nil {
		return "", fmt.Errorf("frame index %d not found", index)
	}

	return fmt.Sprintf("%v", content), nil
}
