// Package browser - anti-detection fingerprint mitigations.
//
// What: Chrome flags and JS patches that reduce bot-detection signal.
// Why:  Headless Chrome leaks obvious fingerprints (navigator.webdriver,
//       missing plugins, CDP-specific properties). Stealth mode patches these.
// How:  Additional ExecAllocator flags + a JS init script injected via CDP.
package browser

import (
	"github.com/chromedp/chromedp"
)

// applyStealthFlags appends anti-fingerprint Chrome launch flags.
//
// These flags match commonly used headless bypass techniques:
//   - Mask headless-mode UA markers.
//   - Disable automation-specific Chrome InfoBars.
//   - Enable GPU compositing to match real browser behaviour.
func applyStealthFlags(opts []chromedp.ExecAllocatorOption) []chromedp.ExecAllocatorOption {
	return append(opts,
		// Why: The "HeadlessChrome" UA string is the simplest bot signal.
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) "+
			"AppleWebKit/537.36 (KHTML, like Gecko) "+
			"Chrome/131.0.0.0 Safari/537.36"),

		// Why: Some detection scripts probe for disabled features.
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),

		// Why: Prevents the DevToolsActivePort file from being written,
		// reducing noise that detection services look for.
		chromedp.Flag("disable-dev-shm-usage", true),

		// Why: Enables the GPU process -- real browsers use it; headless often skips it.
		chromedp.Flag("use-gl", "swiftshader"),

		// Why: Prevent crash reporter noise.
		chromedp.Flag("disable-crash-reporter", true),
	)
}

// StealthInitScript is injected via Page.addScriptToEvaluateOnNewDocument
// to patch JavaScript properties that betray headless Chrome.
const StealthInitScript = `
// Patch 1: Remove navigator.webdriver.
// Why: This is the primary JS-level bot signal. Automation frameworks set it to true.
Object.defineProperty(navigator, 'webdriver', { get: () => undefined });

// Patch 2: Restore plugins array.
// Why: Real browsers have non-empty navigator.plugins. Headless has 0.
Object.defineProperty(navigator, 'plugins', {
  get: () => [
    { name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer', description: 'Portable Document Format', length: 1 },
    { name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai', description: '', length: 1 },
    { name: 'Native Client', filename: 'internal-nacl-plugin', description: '', length: 2 }
  ]
});

// Patch 3: Restore languages.
// Why: Headless Chrome may have an empty or unusual languages array.
Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });

// Patch 4: Chrome runtime.
// Why: Headless Chrome may not have window.chrome defined.
if (!window.chrome) {
  window.chrome = { runtime: {} };
}

// Patch 5: Notification permissions.
// Why: Detection scripts probe permission query behaviour.
const originalQuery = window.navigator.permissions.query;
window.navigator.permissions.query = (parameters) =>
  parameters.name === 'notifications'
    ? Promise.resolve({ state: Notification.permission })
    : originalQuery(parameters);

// Patch 6: Screen dimensions.
// Why: Headless mode reports unusual screen sizes.
Object.defineProperty(window.screen, 'availWidth',  { get: () => 1920 });
Object.defineProperty(window.screen, 'availHeight', { get: () => 1080 });
`
