// Package browser - Cookie and web storage management for AI agents.
//
// What: Read, write, delete cookies and access localStorage/sessionStorage.
// Why:  Agents need to inspect auth state, inject tokens, clear sessions,
//       and read stored preferences. Without cookie/storage access, agents
//       can't debug authentication flows or manipulate state.
// How:  Uses CDP network.GetCookies/SetCookie/DeleteCookies for cookies,
//       and JS eval for localStorage/sessionStorage access.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// CookieEntry represents a browser cookie.
type CookieEntry struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires,omitempty"`
	HTTPOnly bool    `json:"http_only"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"same_site,omitempty"`
}

// StorageEntry is a key-value pair from localStorage or sessionStorage.
type StorageEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GetCookies retrieves all cookies for the current page.
func GetCookies(ctx context.Context) ([]CookieEntry, error) {
	var cookies []*network.Cookie
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().Do(ctx)
			return err
		}),
	); err != nil {
		return nil, fmt.Errorf("get cookies: %w", err)
	}

	entries := make([]CookieEntry, len(cookies))
	for i, c := range cookies {
		entries[i] = CookieEntry{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  c.Expires,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: c.SameSite.String(),
		}
	}
	return entries, nil
}

// SetCookie sets a single cookie.
func SetCookie(ctx context.Context, name, value, domain, path string) error {
	if domain == "" {
		// Use the current page's domain.
		var url string
		chromedp.Run(ctx, chromedp.Location(&url))
	}
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie(name, value).
				WithDomain(domain).
				WithPath(path).
				Do(ctx)
		}),
	)
}

// DeleteCookies deletes cookies by name.
func DeleteCookies(ctx context.Context, name string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.DeleteCookies(name).Do(ctx)
		}),
	)
}

// ClearAllCookies deletes all cookies.
func ClearAllCookies(ctx context.Context) error {
	cookies, err := GetCookies(ctx)
	if err != nil {
		return err
	}
	for _, c := range cookies {
		if err := DeleteCookies(ctx, c.Name); err != nil {
			return err
		}
	}
	return nil
}

// GetStorage retrieves all keys and values from localStorage or sessionStorage.
// storageType should be "local" or "session".
func GetStorage(ctx context.Context, storageType string) ([]StorageEntry, error) {
	jsObj := "localStorage"
	if storageType == "session" {
		jsObj = "sessionStorage"
	}

	js := fmt.Sprintf(`
		(function() {
			const storage = %s;
			const entries = [];
			for (let i = 0; i < storage.length; i++) {
				const key = storage.key(i);
				entries.push({key: key, value: storage.getItem(key)});
			}
			return entries;
		})()
	`, jsObj)

	var entries []StorageEntry
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &entries)); err != nil {
		return nil, fmt.Errorf("get %s storage: %w", storageType, err)
	}
	return entries, nil
}

// SetStorage sets a key-value pair in localStorage or sessionStorage.
func SetStorage(ctx context.Context, storageType, key, value string) error {
	jsObj := "localStorage"
	if storageType == "session" {
		jsObj = "sessionStorage"
	}

	js := fmt.Sprintf(`%s.setItem(%q, %q)`, jsObj, key, value)
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}

// ClearStorage clears all entries from localStorage or sessionStorage.
func ClearStorage(ctx context.Context, storageType string) error {
	jsObj := "localStorage"
	if storageType == "session" {
		jsObj = "sessionStorage"
	}

	js := fmt.Sprintf(`%s.clear()`, jsObj)
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}
