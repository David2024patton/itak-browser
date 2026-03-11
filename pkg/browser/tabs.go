// Package browser - Multi-tab management for browser sessions.
//
// What: Create, switch between, close, and list browser tabs within a session.
// Why:  Agents frequently need to open links in new tabs, compare pages
//       side-by-side, and work across multiple contexts without losing state.
// How:  Uses chromedp.NewContext to create child contexts (tabs) from the
//       parent browser context. Each tab gets its own console/network capture.
package browser

import (
	"context"
	"fmt"
	"sync"

	"github.com/chromedp/chromedp"
)

// Tab represents a single browser tab within a session.
type Tab struct {
	ID     string             `json:"id"`
	URL    string             `json:"url"`
	Title  string             `json:"title"`
	Active bool               `json:"active"`
	ctx    context.Context
	cancel context.CancelFunc
}

// TabManager handles multiple tabs within a single Engine.
type TabManager struct {
	mu       sync.Mutex
	tabs     map[string]*Tab
	activeID string
	counter  int
	parentCtx context.Context
}

// NewTabManager creates a tab manager rooted at the given browser context.
func NewTabManager(parentCtx context.Context) *TabManager {
	return &TabManager{
		tabs:      make(map[string]*Tab),
		parentCtx: parentCtx,
	}
}

// RegisterMain registers the initial browser tab (created during engine launch).
func (tm *TabManager) RegisterMain(ctx context.Context) string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	id := "tab_0"
	tm.tabs[id] = &Tab{
		ID:     id,
		Active: true,
		ctx:    ctx,
	}
	tm.activeID = id
	return id
}

// NewTab opens a new browser tab and optionally navigates to a URL.
func (tm *TabManager) NewTab(url string) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.counter++
	id := fmt.Sprintf("tab_%d", tm.counter)

	ctx, cancel := chromedp.NewContext(tm.parentCtx)

	// Trigger the tab creation.
	if err := chromedp.Run(ctx); err != nil {
		cancel()
		return "", fmt.Errorf("tab new: %w", err)
	}

	if url != "" {
		if err := chromedp.Run(ctx,
			chromedp.Navigate(url),
			chromedp.WaitReady("body", chromedp.ByQuery),
		); err != nil {
			cancel()
			return "", fmt.Errorf("tab new navigate: %w", err)
		}
	}

	// Deactivate current tab.
	if current, ok := tm.tabs[tm.activeID]; ok {
		current.Active = false
	}

	tab := &Tab{
		ID:     id,
		URL:    url,
		Active: true,
		ctx:    ctx,
		cancel: cancel,
	}
	tm.tabs[id] = tab
	tm.activeID = id

	return id, nil
}

// SwitchTab activates a different tab.
func (tm *TabManager) SwitchTab(tabID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tab, ok := tm.tabs[tabID]
	if !ok {
		return fmt.Errorf("tab %q not found", tabID)
	}

	// Deactivate current.
	if current, ok := tm.tabs[tm.activeID]; ok {
		current.Active = false
	}

	tab.Active = true
	tm.activeID = tabID
	return nil
}

// CloseTab closes a specific tab. Cannot close the last remaining tab.
func (tm *TabManager) CloseTab(tabID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(tm.tabs) <= 1 {
		return fmt.Errorf("cannot close the last tab")
	}

	tab, ok := tm.tabs[tabID]
	if !ok {
		return fmt.Errorf("tab %q not found", tabID)
	}

	if tab.cancel != nil {
		tab.cancel()
	}
	delete(tm.tabs, tabID)

	// If we closed the active tab, switch to another.
	if tm.activeID == tabID {
		for id, t := range tm.tabs {
			t.Active = true
			tm.activeID = id
			break
		}
	}

	return nil
}

// ListTabs returns info about all open tabs.
func (tm *TabManager) ListTabs() []Tab {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tabs := make([]Tab, 0, len(tm.tabs))
	for _, t := range tm.tabs {
		// Refresh URL and title from live context.
		var url, title string
		if t.ctx != nil {
			chromedp.Run(t.ctx,
				chromedp.Location(&url),
				chromedp.Title(&title),
			)
			t.URL = url
			t.Title = title
		}
		tabs = append(tabs, Tab{
			ID:     t.ID,
			URL:    t.URL,
			Title:  t.Title,
			Active: t.Active,
		})
	}
	return tabs
}

// ActiveCtx returns the context of the currently active tab.
func (tm *TabManager) ActiveCtx() context.Context {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tab, ok := tm.tabs[tm.activeID]; ok {
		return tab.ctx
	}
	return tm.parentCtx
}

// ActiveID returns the ID of the active tab.
func (tm *TabManager) ActiveID() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.activeID
}
