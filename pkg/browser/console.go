// Package browser - Chrome DevTools console log capture.
//
// What: Captures console.log/warn/error/info output from the browser page.
// Why:  Agents need visibility into page-level errors and debug output to
//       diagnose failures, detect JS exceptions, and verify their eval() calls.
// How:  Enables CDP Runtime domain, listens on ConsoleAPICalled and
//       ExceptionThrown events, stores entries in a capped ring buffer.
package browser

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// ConsoleEntry is a single captured console message.
type ConsoleEntry struct {
	Level     string    `json:"level"`     // "log", "warn", "error", "info", "debug"
	Text      string    `json:"text"`      // Message content
	Timestamp time.Time `json:"timestamp"` // When captured
	Source    string    `json:"source"`    // "console" or "exception"
}

// ConsoleCapture manages a ring buffer of browser console messages.
type ConsoleCapture struct {
	mu      sync.Mutex
	entries []ConsoleEntry
	maxSize int
}

// NewConsoleCapture creates a console capture buffer.
func NewConsoleCapture(maxSize int) *ConsoleCapture {
	if maxSize <= 0 {
		maxSize = 256
	}
	return &ConsoleCapture{
		entries: make([]ConsoleEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add appends a console entry, dropping the oldest if at capacity.
func (cc *ConsoleCapture) Add(entry ConsoleEntry) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if len(cc.entries) >= cc.maxSize {
		cc.entries = cc.entries[1:]
	}
	cc.entries = append(cc.entries, entry)
}

// Entries returns all captured console entries (oldest first).
func (cc *ConsoleCapture) Entries() []ConsoleEntry {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	result := make([]ConsoleEntry, len(cc.entries))
	copy(result, cc.entries)
	return result
}

// Clear empties the console buffer.
func (cc *ConsoleCapture) Clear() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.entries = cc.entries[:0]
}

// EnableConsoleCapture wires up CDP event listeners for console output.
// Must be called after the browser context is created.
func EnableConsoleCapture(ctx context.Context, capture *ConsoleCapture) error {
	// Enable the Runtime domain to receive console events.
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return runtime.Enable().Do(ctx)
		}),
	); err != nil {
		return fmt.Errorf("console: enable runtime: %w", err)
	}

	// Listen for console API calls (console.log, console.warn, etc.)
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			level := string(e.Type)
			parts := make([]string, 0, len(e.Args))
			for _, arg := range e.Args {
				if arg.Value != nil {
					parts = append(parts, strings.Trim(string(arg.Value), "\""))
				} else if arg.Description != "" {
					parts = append(parts, arg.Description)
				} else if arg.UnserializableValue != "" {
					parts = append(parts, string(arg.UnserializableValue))
				}
			}
			capture.Add(ConsoleEntry{
				Level:     level,
				Text:      strings.Join(parts, " "),
				Timestamp: time.Now(),
				Source:    "console",
			})

		case *runtime.EventExceptionThrown:
			text := "unknown exception"
			if e.ExceptionDetails != nil {
				if e.ExceptionDetails.Text != "" {
					text = e.ExceptionDetails.Text
				}
				if e.ExceptionDetails.Exception != nil && e.ExceptionDetails.Exception.Description != "" {
					text = e.ExceptionDetails.Exception.Description
				}
			}
			capture.Add(ConsoleEntry{
				Level:     "error",
				Text:      text,
				Timestamp: time.Now(),
				Source:    "exception",
			})
		}
	})

	return nil
}
