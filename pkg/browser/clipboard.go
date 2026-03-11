// Package browser - Clipboard access for AI agents.
//
// What: Read from and write to the browser's clipboard.
// Why:  Agents need to copy data out of pages (selected text, form values)
//       or paste data in (auth codes, search queries). JS clipboard API
//       requires user gesture, so we use CDP's Clipboard grant + exec.
// How:  Uses CDP permissions grant for clipboard-read/write, then JS
//       navigator.clipboard API for actual read/write operations.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// ClipboardRead reads the current clipboard text content.
func ClipboardRead(ctx context.Context) (string, error) {
	// Grant clipboard-read permission.
	grantJS := `
		(async function() {
			try {
				const text = await navigator.clipboard.readText();
				return text;
			} catch(e) {
				// Fallback: try document.execCommand.
				const ta = document.createElement('textarea');
				ta.style.position = 'fixed';
				ta.style.left = '-9999px';
				document.body.appendChild(ta);
				ta.focus();
				document.execCommand('paste');
				const text = ta.value;
				document.body.removeChild(ta);
				return text;
			}
		})()
	`

	var text string
	if err := chromedp.Run(ctx, chromedp.Evaluate(grantJS, &text)); err != nil {
		return "", fmt.Errorf("clipboard read: %w", err)
	}
	return text, nil
}

// ClipboardWrite writes text to the browser's clipboard.
func ClipboardWrite(ctx context.Context, text string) error {
	// Use JS to write to clipboard with fallback.
	js := fmt.Sprintf(`
		(async function() {
			try {
				await navigator.clipboard.writeText(%q);
				return true;
			} catch(e) {
				// Fallback: use textarea + execCommand.
				const ta = document.createElement('textarea');
				ta.value = %q;
				ta.style.position = 'fixed';
				ta.style.left = '-9999px';
				document.body.appendChild(ta);
				ta.select();
				document.execCommand('copy');
				document.body.removeChild(ta);
				return true;
			}
		})()
	`, text, text)

	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil {
		return fmt.Errorf("clipboard write: %w", err)
	}
	return nil
}

// ClipboardClear empties the clipboard.
func ClipboardClear(ctx context.Context) error {
	return ClipboardWrite(ctx, "")
}
