// Package browser - Geolocation and User-Agent override for AI agents.
//
// What: Spoof GPS coordinates and override the User-Agent string per session.
// Why:  Agents testing geo-targeted content or needing to appear as specific
//       devices/browsers require these overrides. Also useful for avoiding
//       bot detection by rotating user agents.
// How:  Uses CDP Emulation.setGeolocationOverride for GPS and
//       Emulation.setUserAgentOverride for UA string changes.
package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

// GeoLocation represents GPS coordinates.
type GeoLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy"` // Meters
}

// SetGeolocation overrides the browser's reported GPS coordinates.
func SetGeolocation(ctx context.Context, lat, lon, accuracy float64) error {
	if accuracy <= 0 {
		accuracy = 10 // Default 10 meters.
	}
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetGeolocationOverride().
				WithLatitude(lat).
				WithLongitude(lon).
				WithAccuracy(accuracy).
				Do(ctx)
		}),
	)
}

// ClearGeolocation removes the geolocation override.
func ClearGeolocation(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.ClearGeolocationOverride().Do(ctx)
		}),
	)
}

// SetUserAgent overrides the browser's User-Agent string via Emulation domain.
func SetUserAgent(ctx context.Context, userAgent string) error {
	if userAgent == "" {
		return fmt.Errorf("user agent cannot be empty")
	}
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetUserAgentOverride(userAgent).Do(ctx)
		}),
	)
}

// Common user agent presets for convenience.
var UserAgentPresets = map[string]string{
	"chrome-win":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"chrome-mac":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"chrome-linux": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"firefox-win":  "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"safari-mac":   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15",
	"iphone":       "Mozilla/5.0 (iPhone; CPU iPhone OS 18_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Mobile/15E148 Safari/604.1",
	"android":      "Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36",
	"googlebot":    "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
}
