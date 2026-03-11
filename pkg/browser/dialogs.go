// Package browser - Automatic dialog handling for AI agents.
//
// What: Auto-handles JavaScript alert(), confirm(), prompt() dialogs.
// Why:  Without auto-handling, any page that triggers a dialog will freeze
//       the agent -- chromedp actions block until the dialog is dismissed.
//       This is the #1 cause of agent hangs during web browsing.
// How:  Uses CDP page.EventJavascriptDialogOpening listener to automatically
//       accept or dismiss dialogs, capturing their messages for the agent.
package browser

import (
	"context"
	"sync"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// DialogEntry records a captured dialog event.
type DialogEntry struct {
	Type       string `json:"type"`        // alert, confirm, prompt, beforeunload
	Message    string `json:"message"`
	DefaultVal string `json:"default_value,omitempty"` // For prompt() dialogs
	Action     string `json:"action"`      // accepted, dismissed
	Response   string `json:"response,omitempty"` // Value sent to prompt() if any
}

// DialogHandler auto-handles JS dialogs and records them.
type DialogHandler struct {
	mu       sync.RWMutex
	entries  []DialogEntry
	maxSize  int
	autoMode string // "accept" or "dismiss"
	promptResponse string // Default text for prompt() responses
}

// NewDialogHandler creates a handler that auto-accepts dialogs by default.
func NewDialogHandler(maxSize int) *DialogHandler {
	return &DialogHandler{
		entries:  make([]DialogEntry, 0),
		maxSize:  maxSize,
		autoMode: "accept",
	}
}

// SetMode sets auto-handling behavior: "accept" or "dismiss".
func (dh *DialogHandler) SetMode(mode string) {
	dh.mu.Lock()
	defer dh.mu.Unlock()
	if mode == "accept" || mode == "dismiss" {
		dh.autoMode = mode
	}
}

// SetPromptResponse sets the default text response for prompt() dialogs.
func (dh *DialogHandler) SetPromptResponse(text string) {
	dh.mu.Lock()
	defer dh.mu.Unlock()
	dh.promptResponse = text
}

// Entries returns all captured dialog events (thread-safe copy).
func (dh *DialogHandler) Entries() []DialogEntry {
	dh.mu.RLock()
	defer dh.mu.RUnlock()
	result := make([]DialogEntry, len(dh.entries))
	copy(result, dh.entries)
	return result
}

// Clear removes all captured dialog entries.
func (dh *DialogHandler) Clear() {
	dh.mu.Lock()
	defer dh.mu.Unlock()
	dh.entries = dh.entries[:0]
}

// record adds an entry to the ring buffer.
func (dh *DialogHandler) record(entry DialogEntry) {
	dh.mu.Lock()
	defer dh.mu.Unlock()
	if len(dh.entries) >= dh.maxSize {
		dh.entries = dh.entries[1:]
	}
	dh.entries = append(dh.entries, entry)
}

// EnableDialogHandling wires up auto-handling on the given browser context.
// Must be called after the context is created.
func EnableDialogHandling(ctx context.Context, handler *DialogHandler) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *page.EventJavascriptDialogOpening:
			handler.mu.RLock()
			mode := handler.autoMode
			promptResp := handler.promptResponse
			handler.mu.RUnlock()

			accept := mode == "accept"
			entry := DialogEntry{
				Type:       e.Type.String(),
				Message:    e.Message,
				DefaultVal: e.DefaultPrompt,
			}

			if accept {
				entry.Action = "accepted"
				entry.Response = promptResp
			} else {
				entry.Action = "dismissed"
			}

			handler.record(entry)

			// Handle the dialog via CDP.
			go chromedp.Run(ctx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					return page.HandleJavaScriptDialog(accept).
						WithPromptText(promptResp).
						Do(ctx)
				}),
			)
		}
	})
}
