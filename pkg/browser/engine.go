// Package browser implements the iTaK BrowserEngine contract using chromedp.
//
// What: A CDP-native browser automation engine for AI agents.
// Why:  Agents need to interact with the web without spawning Node.js/Playwright.
//       chromedp gives us a pure-Go path to Chrome via the DevTools Protocol.
// How:  Each Engine wraps a chromedp browser context. Sessions persist across
//       commands via a long-lived ExecAllocator. Snapshots reduce DOM tokens by 93%.
package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/cdproto/dom"
	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const (
	// ModuleName is emitted as the source in events.
	ModuleName = "browser"
	// Version of the iTaK Browser module.
	Version = "0.2.3"
)

// Engine implements the BrowserEngine contract using chromedp.
// It manages a single persistent Chrome instance and many logical sessions.
type Engine struct {
	mu sync.Mutex

	sessionID  string
	profileDir string
	headless   bool
	stealth    bool

	// allocCtx is the parent context for the Chrome allocator.
	allocCtx    context.Context
	allocCancel context.CancelFunc

	// browserCtx is the context for the browser itself.
	browserCtx    context.Context
	browserCancel context.CancelFunc

	// eventFn is called whenever a browser event fires (navigate, action, error).
	eventFn func(typ, source string, data any)

	// Console and network monitoring.
	console *ConsoleCapture
	network *NetworkCapture

	// ContentGuard scans extracted content for AI recommendation poisoning.
	guard *ContentGuard

	// Tab management.
	tabs *TabManager

	// Request blocking (ads, trackers).
	blocker *RequestBlocker

	// Dialog auto-handling.
	dialogs *DialogHandler

	// Download tracking.
	downloads *DownloadTracker

	// v0.2.3: Teaching + discovery.
	recorder *ActionRecorder
	history  *NavigationHistory

	// v0.3.0: Screen recording state.
	recording *RecordingState

	startTime time.Time
	logger    *slog.Logger
}

// Options configures an Engine at creation time.
type Options struct {
	// SessionID uniquely identifies this browser session (default: random UUID).
	SessionID string
	// ProfileDir is where cookies/storage persist (default: OS temp).
	ProfileDir string
	// Headless controls visible vs. headless mode (default: true).
	// Set to false for headed (visible Chrome window) mode.
	Headless bool
	// Stealth enables anti-detection fingerprint mitigations (default: false).
	Stealth bool
	// WindowWidth is the viewport width in pixels (default: 1920).
	WindowWidth int
	// WindowHeight is the viewport height in pixels (default: 1080).
	WindowHeight int
	// ProxyServer is an optional HTTP/SOCKS proxy (e.g., "http://proxy:8080").
	ProxyServer string
	// BlockAds enables default ad/tracker blocking (default: false).
	BlockAds bool
	// EventFn receives browser events for the Core event bus integration.
	EventFn func(typ, source string, data any)
	// Logger is used for structured output (default: slog.Default()).
	Logger *slog.Logger
}

// New creates a new Engine and launches the Chrome instance.
// The caller must call Close() when done to release resources.
func New(opts Options) (*Engine, error) {
	// What: Apply defaults for any unset options.
	if opts.SessionID == "" {
		opts.SessionID = fmt.Sprintf("ses_%d", time.Now().UnixNano())
	}
	if opts.ProfileDir == "" {
		opts.ProfileDir = filepath.Join(os.TempDir(), "itak-browser", opts.SessionID)
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.EventFn == nil {
		opts.EventFn = func(typ, source string, data any) {}
	}
	if opts.WindowWidth <= 0 {
		opts.WindowWidth = 1920
	}
	if opts.WindowHeight <= 0 {
		opts.WindowHeight = 1080
	}

	// Why: Persist the user data dir so cookies/session survive daemon restarts.
	if err := os.MkdirAll(opts.ProfileDir, 0700); err != nil {
		return nil, fmt.Errorf("browser: create profile dir: %w", err)
	}

	e := &Engine{
		sessionID:  opts.SessionID,
		profileDir: opts.ProfileDir,
		headless:   opts.Headless,
		stealth:    opts.Stealth,
		eventFn:    opts.EventFn,
		console:    NewConsoleCapture(256),
		network:    NewNetworkCapture(512),
		guard:      NewContentGuard(),
		dialogs:    NewDialogHandler(128),
		downloads:  NewDownloadTracker(filepath.Join(opts.ProfileDir, "downloads")),
		recorder:   NewActionRecorder(),
		history:    NewNavigationHistory(),
		startTime:  time.Now(),
		logger:     opts.Logger,
	}

	if opts.BlockAds {
		e.blocker = NewRequestBlocker()
	}

	if err := e.launch(opts); err != nil {
		return nil, err
	}

	// What: Auto-inject the annotation toolbar in headed mode.
	// Why:  Users expect the toolbar to appear immediately when the
	//       visible browser window opens, not require a second command.
	// How:  Register the toolbar JS via addScriptToEvaluateOnNewDocument
	//       so it fires on every page load, surviving navigations.
	//       A background goroutine also pre-captures a CDP screenshot
	//       after each page load so the Capture button has real content.
	if !e.headless {
		if err := chromedp.Run(e.browserCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				// Wrap toolbar JS in a DOMContentLoaded listener so it
				// runs after the DOM is ready on each navigation.
				wrapped := `document.addEventListener('DOMContentLoaded', function() {
					if (document.getElementById('itak-toolbar')) return;
					` + toolbarJS + `
				}, {once: true});
				// Also run immediately if DOM is already loaded (e.g. blank tab).
				if (document.readyState !== 'loading') {
					if (!document.getElementById('itak-toolbar')) {
						` + toolbarJS + `
					}
				}`
				_, err := page.AddScriptToEvaluateOnNewDocument(wrapped).Do(ctx)
				return err
			}),
		); err != nil {
			e.logger.Warn("auto-inject toolbar registration failed", "err", err)
		}

		// What: Pre-capture a CDP screenshot after each page load.
		// Why:  The Capture button needs a real page image. External JS
		//       libraries (html2canvas) are blocked by CSP on many sites.
		//       CDP screenshots always work and are pixel-perfect.
		go e.watchToolbarRequests()
	}

	return e, nil
}

// watchToolbarRequests polls for toolbar capture and save requests.
// Capture: takes a CDP screenshot and injects it for the toolbar.
// Save: reads annotated canvas data URL, writes PNG to disk, injects path.
func (e *Engine) watchToolbarRequests() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	// Create screenshots directory.
	home, _ := os.UserHomeDir()
	screenshotDir := filepath.Join(home, "Pictures", "iTaK Screenshots")
	os.MkdirAll(screenshotDir, 0755)

	for {
		select {
		case <-e.browserCtx.Done():
			return
		case <-ticker.C:
			// ---- Handle capture request ----
			var captureReq bool
			if err := chromedp.Run(e.browserCtx,
				chromedp.Evaluate(`!!window.__itak_capture_requested`, &captureReq),
			); err != nil {
				continue
			}
			if captureReq {
				chromedp.Run(e.browserCtx, chromedp.Evaluate(`window.__itak_capture_requested = false`, nil)) //nolint:errcheck

				var buf []byte
				if err := chromedp.Run(e.browserCtx, chromedp.CaptureScreenshot(&buf)); err == nil {
					js := fmt.Sprintf(`window.__itak_cdp_screenshot = "data:image/png;base64,%s"`,
						base64Encode(buf))
					chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil)) //nolint:errcheck
				}
			}

			// ---- Handle save request ----
			var saveReq bool
			if err := chromedp.Run(e.browserCtx,
				chromedp.Evaluate(`!!window.__itak_save_requested`, &saveReq),
			); err != nil || !saveReq {
				continue
			}

			chromedp.Run(e.browserCtx, chromedp.Evaluate(`window.__itak_save_requested = false`, nil)) //nolint:errcheck

			// Read the data URL from JS.
			var dataURL string
			if err := chromedp.Run(e.browserCtx,
				chromedp.Evaluate(`window.__itak_save_data || ""`, &dataURL),
			); err != nil || dataURL == "" {
				continue
			}

			// Strip the data:image/png;base64, prefix.
			idx := 0
			for i := 0; i < len(dataURL); i++ {
				if dataURL[i] == ',' {
					idx = i + 1
					break
				}
			}
			if idx == 0 {
				continue
			}

			// Decode and write.
			raw := dataURL[idx:]
			decoded, err := base64Decode(raw)
			if err != nil {
				continue
			}

			fname := fmt.Sprintf("itak-screenshot-%d.png", time.Now().UnixMilli())
			fpath := filepath.Join(screenshotDir, fname)
			if err := os.WriteFile(fpath, decoded, 0644); err != nil {
				continue
			}

			// Inject the saved path for clipboard.
			pathJS := fmt.Sprintf(`window.__itak_saved_path = %q`, fpath)
			chromedp.Run(e.browserCtx, chromedp.Evaluate(pathJS, nil)) //nolint:errcheck
			chromedp.Run(e.browserCtx, chromedp.Evaluate(`window.__itak_save_data = null`, nil)) //nolint:errcheck
		}
	}
}

// base64Decode decodes a base64 string to bytes.
func base64Decode(s string) ([]byte, error) {
	const table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var lookup [256]byte
	for i := range lookup {
		lookup[i] = 0xff
	}
	for i, c := range table {
		lookup[c] = byte(i)
	}
	lookup['='] = 0

	// Strip whitespace.
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if lookup[s[i]] != 0xff || s[i] == '=' {
			clean = append(clean, s[i])
		}
	}

	out := make([]byte, 0, len(clean)*3/4)
	for i := 0; i < len(clean); i += 4 {
		if i+3 >= len(clean) {
			break
		}
		a := lookup[clean[i]]
		b := lookup[clean[i+1]]
		c := lookup[clean[i+2]]
		d := lookup[clean[i+3]]
		out = append(out, (a<<2)|(b>>4))
		if clean[i+2] != '=' {
			out = append(out, (b<<4)|(c>>2))
		}
		if clean[i+3] != '=' {
			out = append(out, (c<<6)|d)
		}
	}
	return out, nil
}

// launch starts the Chrome allocator and browser context.
func (e *Engine) launch(opts Options) error {
	// How: Build Chrome flags. chromedp.NewExecAllocator wraps chrome/chromium.
	flags := buildFlags(e.headless, e.stealth, e.profileDir, opts.WindowWidth, opts.WindowHeight, opts.ProxyServer)

	e.allocCtx, e.allocCancel = chromedp.NewExecAllocator(context.Background(), flags...)
	e.browserCtx, e.browserCancel = chromedp.NewContext(e.allocCtx,
		chromedp.WithLogf(func(format string, args ...interface{}) {
			e.logger.Debug(fmt.Sprintf("[chromedp] "+format, args...))
		}),
	)

	// Trigger browser startup by running a no-op action.
	if err := chromedp.Run(e.browserCtx); err != nil {
		return fmt.Errorf("browser: launch chrome: %w", err)
	}

	// Wire up stealth init script injection via CDP.
	// Why: The StealthInitScript patches JS properties on every new document,
	//      so it must be injected via Page.addScriptToEvaluateOnNewDocument
	//      which runs before page JS executes.
	if e.stealth {
		if err := chromedp.Run(e.browserCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				_, err := page.AddScriptToEvaluateOnNewDocument(StealthInitScript).Do(ctx)
				return err
			}),
		); err != nil {
			e.logger.Warn("stealth: failed to inject init script", "err", err)
		}
	}

	// Enable console and network monitoring.
	if err := EnableConsoleCapture(e.browserCtx, e.console); err != nil {
		e.logger.Warn("console capture failed", "err", err)
	}
	if err := EnableNetworkCapture(e.browserCtx, e.network); err != nil {
		e.logger.Warn("network capture failed", "err", err)
	}

	// Enable request blocking if configured.
	if e.blocker != nil {
		if err := EnableRequestBlocking(e.browserCtx, e.blocker); err != nil {
			e.logger.Warn("request blocking failed", "err", err)
		}
	}

	// Initialize tab manager.
	e.tabs = NewTabManager(e.allocCtx)
	e.tabs.RegisterMain(e.browserCtx)

	// Enable dialog auto-handling.
	if e.dialogs != nil {
		EnableDialogHandling(e.browserCtx, e.dialogs)
	}

	// Enable download tracking.
	if e.downloads != nil {
		if err := EnableDownloadTracking(e.browserCtx, e.downloads); err != nil {
			e.logger.Warn("download tracking failed", "err", err)
		}
	} else if !e.headless {
		// Even without a tracker, enable downloads in headed mode so toolbar
		// Save button works. Use the user's default Downloads folder.
		dlDir := ""
		if home, err := os.UserHomeDir(); err == nil {
			dlDir = filepath.Join(home, "Downloads")
		}
		chromedp.Run(e.browserCtx, //nolint:errcheck
			chromedp.ActionFunc(func(ctx context.Context) error {
				return cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorAllow).
					WithDownloadPath(dlDir).
					WithEventsEnabled(false).
					Do(ctx)
			}),
		)
	}

	mode := "headless"
	if !e.headless {
		mode = "headed"
	}
	e.logger.Info("browser launched", "session", e.sessionID, "mode", mode,
		"viewport", fmt.Sprintf("%dx%d", opts.WindowWidth, opts.WindowHeight))
	e.eventFn("browser.started", ModuleName, map[string]string{
		"session": e.sessionID, "mode": mode,
	})
	return nil
}

// ---- Navigation ----

// Open navigates to a URL and waits for the page body to be ready.
func (e *Engine) Open(ctx context.Context, url string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.logger.Info("open", "url", url)

	err := chromedp.Run(e.browserCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
	)
	if err != nil {
		e.eventFn("browser.error", ModuleName, map[string]string{"op": "open", "url": url, "err": err.Error()})
		return fmt.Errorf("open %s: %w", url, err)
	}

	e.eventFn("browser.navigate", ModuleName, map[string]string{"url": url, "session": e.sessionID})
	return nil
}

// Back navigates back in browser history.
func (e *Engine) Back(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return chromedp.Run(e.browserCtx, chromedp.NavigateBack())
}

// Forward navigates forward in browser history.
func (e *Engine) Forward(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return chromedp.Run(e.browserCtx, chromedp.NavigateForward())
}

// Reload refreshes the current page.
func (e *Engine) Reload(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return chromedp.Run(e.browserCtx, chromedp.Reload())
}

// ---- Interaction (v0.1) ----

// Click clicks the element identified by ref ID.
func (e *Engine) Click(ctx context.Context, ref string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return err
	}

	if err := chromedp.Run(e.browserCtx,
		chromedp.Click(sel, chromedp.ByQuery),
	); err != nil {
		e.eventFn("browser.error", ModuleName, map[string]string{"op": "click", "ref": ref, "err": err.Error()})
		return fmt.Errorf("click %s: %w", ref, err)
	}

	e.eventFn("browser.action", ModuleName, map[string]string{"op": "click", "ref": ref})
	return nil
}

// Fill types text into a form element by ref ID, clearing existing value first.
func (e *Engine) Fill(ctx context.Context, ref string, text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return err
	}

	if err := chromedp.Run(e.browserCtx,
		chromedp.Clear(sel, chromedp.ByQuery),
		chromedp.SendKeys(sel, text, chromedp.ByQuery),
	); err != nil {
		e.eventFn("browser.error", ModuleName, map[string]string{"op": "fill", "ref": ref, "err": err.Error()})
		return fmt.Errorf("fill %s: %w", ref, err)
	}

	e.eventFn("browser.action", ModuleName, map[string]string{"op": "fill", "ref": ref})
	return nil
}

// Press sends a key press to the page (e.g., "Enter", "Tab", "Escape").
func (e *Engine) Press(ctx context.Context, key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return chromedp.Run(e.browserCtx, chromedp.KeyEvent(key))
}

// Scroll scrolls the page by delta pixels.
func (e *Engine) Scroll(ctx context.Context, deltaX, deltaY float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	js := fmt.Sprintf("window.scrollBy(%f, %f)", deltaX, deltaY)
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}

// Eval executes JavaScript in the page context and returns the stringified result.
func (e *Engine) Eval(ctx context.Context, js string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var result interface{}
	if err := chromedp.Run(e.browserCtx,
		chromedp.Evaluate(js, &result),
	); err != nil {
		return "", fmt.Errorf("eval: %w", err)
	}

	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result), nil
	}
	return string(out), nil
}

// ---- Interaction (v0.2 - headed mode expansion) ----

// DblClick double-clicks the element identified by ref ID.
func (e *Engine) DblClick(ctx context.Context, ref string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return err
	}
	return chromedp.Run(e.browserCtx, chromedp.DoubleClick(sel, chromedp.ByQuery))
}

// Hover moves the mouse over the element identified by ref ID.
func (e *Engine) Hover(ctx context.Context, ref string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return err
	}
	// Why: chromedp doesn't have a direct Hover. We use MouseClickXY on the
	// element center without actually clicking, but simpler: just scroll into
	// view then dispatch a mousemove via JS.
	return chromedp.Run(e.browserCtx,
		chromedp.ScrollIntoView(sel, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Use chromedp to get the element's bounding box and dispatch mouseover.
			js := fmt.Sprintf(`
				(function() {
					const el = document.querySelector(%q);
					if (!el) return false;
					const rect = el.getBoundingClientRect();
					const evt = new MouseEvent('mouseover', {
						bubbles: true, clientX: rect.x + rect.width/2, clientY: rect.y + rect.height/2
					});
					el.dispatchEvent(evt);
					return true;
				})()
			`, sel)
			var ok bool
			return chromedp.Evaluate(js, &ok).Do(ctx)
		}),
	)
}

// Select sets the value of a <select> element by ref ID.
func (e *Engine) Select(ctx context.Context, ref string, values []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return err
	}
	return chromedp.Run(e.browserCtx, chromedp.SetValue(sel, values[0], chromedp.ByQuery))
}

// Check checks a checkbox or radio button by ref ID.
func (e *Engine) Check(ctx context.Context, ref string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return err
	}
	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			if (el && !el.checked) { el.click(); }
			return el ? el.checked : false;
		})()
	`, sel)
	var checked bool
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &checked))
}

// Uncheck unchecks a checkbox by ref ID.
func (e *Engine) Uncheck(ctx context.Context, ref string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return err
	}
	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			if (el && el.checked) { el.click(); }
			return el ? !el.checked : false;
		})()
	`, sel)
	var unchecked bool
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &unchecked))
}

// Upload sets a file path on an <input type="file"> by ref ID.
func (e *Engine) Upload(ctx context.Context, ref string, path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return err
	}
	return chromedp.Run(e.browserCtx, chromedp.SetUploadFiles(sel, []string{path}, chromedp.ByQuery))
}

// MouseMove moves the mouse to pixel coordinates.
func (e *Engine) MouseMove(ctx context.Context, x, y float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return chromedp.Run(e.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(ctx)
		}),
	)
}

// MouseClick clicks at pixel coordinates with the specified button.
func (e *Engine) MouseClick(ctx context.Context, x, y float64, button string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	btn := input.MouseButton("left")
	switch button {
	case "right":
		btn = input.MouseButton("right")
	case "middle":
		btn = input.MouseButton("middle")
	}

	return chromedp.Run(e.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := input.DispatchMouseEvent(input.MousePressed, x, y).
				WithButton(btn).WithClickCount(1).Do(ctx); err != nil {
				return err
			}
			return input.DispatchMouseEvent(input.MouseReleased, x, y).
				WithButton(btn).WithClickCount(1).Do(ctx)
		}),
	)
}

// Resize changes the browser viewport dimensions on the fly.
func (e *Engine) Resize(ctx context.Context, width, height int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := fmt.Sprintf("window.resizeTo(%d, %d)", width, height)
	return chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}

// ---- Snapshot / Capture ----

// Snapshot returns the current page as a compact accessibility tree with ref IDs.
func (e *Engine) Snapshot(ctx context.Context) (Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotLocked()
}

func (e *Engine) snapshotLocked() (Snapshot, error) {
	var title, url string
	if err := chromedp.Run(e.browserCtx,
		chromedp.Title(&title),
		chromedp.Location(&url),
	); err != nil {
		return Snapshot{}, fmt.Errorf("snapshot: get page info: %w", err)
	}

	refs, tree, err := BuildAccessibilityTree(e.browserCtx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("snapshot: build tree: %w", err)
	}

	e.storeRefs(refs)

	return Snapshot{
		URL:   url,
		Title: title,
		Tree:  tree,
		Refs:  refs,
	}, nil
}

func (e *Engine) snapshotRefs() (map[string]Ref, error) {
	snap, err := e.snapshotLocked()
	return snap.Refs, err
}

// Screenshot captures the current viewport.
func (e *Engine) Screenshot(ctx context.Context, annotate bool) (ScreenshotResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	path := filepath.Join(e.profileDir, fmt.Sprintf("screenshot_%d.png", time.Now().UnixNano()))

	var buf []byte
	if err := chromedp.Run(e.browserCtx,
		chromedp.FullScreenshot(&buf, 90),
	); err != nil {
		return ScreenshotResult{}, fmt.Errorf("screenshot: %w", err)
	}

	result := ScreenshotResult{Path: path, Annotated: annotate}

	if annotate {
		refs, err := e.snapshotRefs()
		if err == nil {
			annotated, w, h, err := AnnotateScreenshot(buf, refs)
			if err == nil {
				buf = annotated
				result.Width = w
				result.Height = h
			}
		}
	}

	if err := os.WriteFile(path, buf, 0644); err != nil {
		return ScreenshotResult{}, fmt.Errorf("screenshot save: %w", err)
	}

	e.eventFn("browser.screenshot", ModuleName, map[string]string{"path": path})
	return result, nil
}

// PDF captures the current page as a PDF file.
func (e *Engine) PDF(ctx context.Context, path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var buf []byte
	if err := chromedp.Run(e.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, _, err = page.PrintToPDF().Do(ctx)
			return err
		}),
	); err != nil {
		return fmt.Errorf("pdf: %w", err)
	}

	return os.WriteFile(path, buf, 0644)
}

// ---- Data Extraction (v0.1) ----

// GetText returns visible text of an element by ref.
func (e *Engine) GetText(ctx context.Context, ref string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return "", err
	}
	var text string
	if err := chromedp.Run(e.browserCtx, chromedp.Text(sel, &text, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("get text %s: %w", ref, err)
	}
	return text, nil
}

// GetHTML returns the outer HTML of an element by ref.
func (e *Engine) GetHTML(ctx context.Context, ref string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return "", err
	}
	var html string
	if err := chromedp.Run(e.browserCtx, chromedp.OuterHTML(sel, &html, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("get html %s: %w", ref, err)
	}
	return html, nil
}

// GetTitle returns the current page title.
func (e *Engine) GetTitle(ctx context.Context) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var title string
	return title, chromedp.Run(e.browserCtx, chromedp.Title(&title))
}

// GetURL returns the current page URL.
func (e *Engine) GetURL(ctx context.Context) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var url string
	return url, chromedp.Run(e.browserCtx, chromedp.Location(&url))
}

// WaitVisible waits until a CSS selector is visible on the page.
func (e *Engine) WaitVisible(ctx context.Context, sel string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return chromedp.Run(e.browserCtx, chromedp.WaitVisible(sel, chromedp.ByQuery))
}

// ---- Data Extraction (v0.2 - agent debug features) ----

// GetValue returns the .value property of a form element by ref.
func (e *Engine) GetValue(ctx context.Context, ref string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return "", err
	}
	var value string
	if err := chromedp.Run(e.browserCtx, chromedp.Value(sel, &value, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("get value %s: %w", ref, err)
	}
	return value, nil
}

// GetAttribute returns any HTML attribute of an element by ref.
func (e *Engine) GetAttribute(ctx context.Context, ref, attr string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return "", err
	}
	var value string
	var ok bool
	if err := chromedp.Run(e.browserCtx, chromedp.AttributeValue(sel, attr, &value, &ok, chromedp.ByQuery)); err != nil {
		return "", fmt.Errorf("get attr %s.%s: %w", ref, attr, err)
	}
	if !ok {
		return "", nil // Attribute does not exist.
	}
	return value, nil
}

// BoundingBox holds pixel coordinates and dimensions of an element.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// GetBoundingBox returns the pixel bounding box of an element by ref.
func (e *Engine) GetBoundingBox(ctx context.Context, ref string) (BoundingBox, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return BoundingBox{}, err
	}

	// How: We query the DOM for the node, then use dom.GetBoxModel.
	var nodeIDs []int64
	if err := chromedp.Run(e.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			docNode, err := dom.GetDocument().Do(ctx)
			if err != nil {
				return err
			}
			ids, err := dom.QuerySelectorAll(docNode.NodeID, sel).Do(ctx)
			if err != nil {
				return err
			}
			for _, id := range ids {
				nodeIDs = append(nodeIDs, int64(id))
			}
			return nil
		}),
	); err != nil {
		return BoundingBox{}, fmt.Errorf("get box %s: dom query: %w", ref, err)
	}

	if len(nodeIDs) == 0 {
		return BoundingBox{}, fmt.Errorf("get box %s: element not found", ref)
	}

	// Fallback: Use JS getBoundingClientRect which is more reliable.
	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			if (!el) return null;
			const r = el.getBoundingClientRect();
			return { x: r.x, y: r.y, width: r.width, height: r.height };
		})()
	`, sel)

	var box BoundingBox
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &box)); err != nil {
		return BoundingBox{}, fmt.Errorf("get box %s: %w", ref, err)
	}
	return box, nil
}

// IsVisible returns true if the element is visible in the viewport.
func (e *Engine) IsVisible(ctx context.Context, ref string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return false, err
	}

	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			if (!el) return false;
			const style = window.getComputedStyle(el);
			return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0';
		})()
	`, sel)

	var visible bool
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &visible)); err != nil {
		return false, err
	}
	return visible, nil
}

// IsEnabled returns true if the element is not disabled.
func (e *Engine) IsEnabled(ctx context.Context, ref string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return false, err
	}

	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			return el ? !el.disabled : false;
		})()
	`, sel)

	var enabled bool
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &enabled)); err != nil {
		return false, err
	}
	return enabled, nil
}

// IsChecked returns true if a checkbox/radio is checked.
func (e *Engine) IsChecked(ctx context.Context, ref string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sel, err := e.resolveRef(ref)
	if err != nil {
		return false, err
	}

	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			return el ? !!el.checked : false;
		})()
	`, sel)

	var checked bool
	if err := chromedp.Run(e.browserCtx, chromedp.Evaluate(js, &checked)); err != nil {
		return false, err
	}
	return checked, nil
}

// ---- Console / Network / Debug ----

// ConsoleMessages returns all captured console log entries.
func (e *Engine) ConsoleMessages() []ConsoleEntry {
	return e.console.Entries()
}

// NetworkRequests returns all captured network request/response entries.
func (e *Engine) NetworkRequests() []NetworkEntry {
	return e.network.Entries()
}

// DebugBundle is the all-in-one debug response for agents.
type DebugBundle struct {
	URL            string            `json:"url"`
	Title          string            `json:"title"`
	Tree           string            `json:"tree"`
	Refs           map[string]Ref    `json:"refs"`
	ScreenshotPath string            `json:"screenshot_path"`
	Console        []ConsoleEntry    `json:"console"`
	Network        []NetworkEntry    `json:"network"`
	ScanReport     *ContentGuardResult `json:"scan_report,omitempty"`
}

// Debug produces a single-call "kitchen sink" response: snapshot + screenshot
// + console + network. Agents that want full context in one round-trip use this.
func (e *Engine) Debug(ctx context.Context) (DebugBundle, error) {
	// Take snapshot and screenshot inside the lock.
	e.mu.Lock()

	snap, err := e.snapshotLocked()
	if err != nil {
		e.mu.Unlock()
		return DebugBundle{}, fmt.Errorf("debug: snapshot: %w", err)
	}

	path := filepath.Join(e.profileDir, fmt.Sprintf("debug_%d.png", time.Now().UnixNano()))
	var buf []byte
	if err := chromedp.Run(e.browserCtx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		e.mu.Unlock()
		return DebugBundle{}, fmt.Errorf("debug: screenshot: %w", err)
	}
	e.mu.Unlock()

	if err := os.WriteFile(path, buf, 0644); err != nil {
		return DebugBundle{}, fmt.Errorf("debug: save screenshot: %w", err)
	}

	// Run ContentGuard scan on the snapshot tree.
	var scanReport *ContentGuardResult
	if e.guard != nil {
		result := e.guard.ScanContent(snap.Tree)
		if !result.Clean {
			e.logger.Warn("contentguard: poisoning detected in page content",
				"url", snap.URL, "warnings", len(result.Warnings))
		}
		scanReport = &result
	}

	return DebugBundle{
		URL:            snap.URL,
		Title:          snap.Title,
		Tree:           snap.Tree,
		Refs:           snap.Refs,
		ScreenshotPath: path,
		Console:        e.console.Entries(),
		Network:        e.network.Entries(),
		ScanReport:     scanReport,
	}, nil
}

// ---- Session lifecycle ----

// Close shuts down the Chrome instance and releases all resources.
func (e *Engine) Close(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.logger.Info("closing browser session", "session", e.sessionID)
	e.eventFn("browser.closed", ModuleName, map[string]string{"session": e.sessionID})

	if e.browserCancel != nil {
		e.browserCancel()
	}
	if e.allocCancel != nil {
		e.allocCancel()
	}
	return nil
}

// SessionID returns the unique identifier for this browser session.
func (e *Engine) SessionID() string {
	return e.sessionID
}

// Uptime returns how long this engine has been running.
func (e *Engine) Uptime() time.Duration {
	return time.Since(e.startTime)
}

// IsHeaded returns true if this session is running in visible mode.
func (e *Engine) IsHeaded() bool {
	return !e.headless
}

// ---- Ref management (internal) ----

var refStore sync.Map

func (e *Engine) storeRefs(refs map[string]Ref) {
	m := make(map[string]string, len(refs))
	for id, ref := range refs {
		m[id] = ref.Selector
	}
	refStore.Store(e.sessionID, m)
}

func (e *Engine) resolveRef(ref string) (string, error) {
	v, ok := refStore.Load(e.sessionID)
	if !ok {
		return "", fmt.Errorf("no snapshot taken for session %s -- call snapshot first", e.sessionID)
	}
	m := v.(map[string]string)
	sel, ok := m[ref]
	if !ok {
		return "", fmt.Errorf("unknown ref %q -- call snapshot to refresh refs", ref)
	}
	return sel, nil
}

// ---- v0.2.1: Auto-scan, Alerts, Tabs, Metrics, Extract, Wait ----

// ScanResult is returned by AutoScan to the daemon/CLI.
type ScanResult struct {
	Clean    bool     `json:"clean"`
	Warnings []string `json:"warnings,omitempty"`
	Alerted  bool     `json:"alerted"` // True if a headed-mode popup was shown
}

// AutoScan runs ContentGuard on the current page and optionally triggers
// a headed-mode visual alert if poisoning is detected.
// This is called automatically after Open and Snapshot by the daemon.
func (e *Engine) AutoScan(ctx context.Context) ScanResult {
	if e.guard == nil {
		return ScanResult{Clean: true}
	}

	// Get the page text for scanning.
	var pageText string
	chromedp.Run(e.browserCtx,
		chromedp.Evaluate(`document.body ? document.body.innerText : ''`, &pageText),
	)

	// Also scan the current URL.
	var currentURL string
	chromedp.Run(e.browserCtx, chromedp.Location(&currentURL))

	contentResult := e.guard.ScanContent(pageText)
	urlResult := e.guard.ScanURL(currentURL)

	result := ScanResult{Clean: contentResult.Clean && urlResult.Clean}
	result.Warnings = append(result.Warnings, contentResult.Warnings...)
	result.Warnings = append(result.Warnings, urlResult.Warnings...)

	if !result.Clean {
		e.logger.Warn("contentguard: poisoning detected",
			"url", currentURL, "warnings", len(result.Warnings))

		// Headed-mode popup alert.
		if !e.headless {
			e.showPoisonAlert(currentURL, result.Warnings)
			result.Alerted = true
		}
	}

	return result
}

// showPoisonAlert injects a visual warning overlay into the headed browser window.
// What: A fixed-position overlay with a red warning banner.
// Why:  In headed mode, the user watching the browser needs to see the threat immediately.
// How:  JS injection that creates a DOM overlay, auto-dismisses after 15 seconds.
func (e *Engine) showPoisonAlert(url string, warnings []string) {
	warningText := ""
	for i, w := range warnings {
		if i >= 5 {
			warningText += fmt.Sprintf("\\n... and %d more warnings", len(warnings)-5)
			break
		}
		// Escape single quotes for JS.
		escaped := ""
		for _, ch := range w {
			if ch == '\'' {
				escaped += "\\'"
			} else if ch == '\n' {
				escaped += "\\n"
			} else {
				escaped += string(ch)
			}
		}
		warningText += escaped + "\\n"
	}

	js := fmt.Sprintf(`
		(function() {
			// Remove any existing alert.
			const existing = document.getElementById('itak-poison-alert');
			if (existing) existing.remove();

			const overlay = document.createElement('div');
			overlay.id = 'itak-poison-alert';
			overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;z-index:999999;' +
				'background:linear-gradient(135deg,#dc2626,#991b1b);color:white;' +
				'padding:16px 24px;font-family:system-ui,sans-serif;font-size:14px;' +
				'box-shadow:0 4px 24px rgba(0,0,0,0.4);border-bottom:3px solid #fbbf24;' +
				'cursor:pointer;';

			overlay.innerHTML = '<div style="display:flex;align-items:center;gap:12px">' +
				'<span style="font-size:28px">⚠️</span>' +
				'<div>' +
				'<div style="font-weight:bold;font-size:16px;margin-bottom:4px">' +
				'iTaK ContentGuard: AI Recommendation Poisoning Detected</div>' +
				'<div style="opacity:0.9;font-size:12px">%s</div>' +
				'<div style="opacity:0.7;font-size:11px;margin-top:4px">Click to dismiss | Auto-closes in 15s</div>' +
				'</div></div>';

			overlay.onclick = function() { overlay.remove(); };
			document.body.appendChild(overlay);

			// Auto-dismiss after 15 seconds.
			setTimeout(function() {
				if (overlay.parentNode) overlay.remove();
			}, 15000);
		})()
	`, warningText)

	chromedp.Run(e.browserCtx, chromedp.Evaluate(js, nil))
}

// ---- Tab Management ----

// TabNew opens a new tab, optionally navigating to a URL.
func (e *Engine) TabNew(ctx context.Context, url string) (string, error) {
	if e.tabs == nil {
		return "", fmt.Errorf("tab manager not initialized")
	}
	return e.tabs.NewTab(url)
}

// TabSwitch activates a different tab.
func (e *Engine) TabSwitch(ctx context.Context, tabID string) error {
	if e.tabs == nil {
		return fmt.Errorf("tab manager not initialized")
	}
	return e.tabs.SwitchTab(tabID)
}

// TabClose closes a tab.
func (e *Engine) TabClose(ctx context.Context, tabID string) error {
	if e.tabs == nil {
		return fmt.Errorf("tab manager not initialized")
	}
	return e.tabs.CloseTab(tabID)
}

// TabList returns all open tabs.
func (e *Engine) TabList(ctx context.Context) []Tab {
	if e.tabs == nil {
		return nil
	}
	return e.tabs.ListTabs()
}

// ---- Page Metrics ----

// Metrics returns performance data about the current page.
func (e *Engine) Metrics(ctx context.Context) (PageMetrics, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return CollectPageMetrics(e.browserCtx)
}

// ---- Bulk Extraction ----

// Links returns all hyperlinks from the current page.
func (e *Engine) Links(ctx context.Context) ([]PageLink, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ExtractLinks(e.browserCtx)
}

// Forms returns all forms and their fields from the current page.
func (e *Engine) Forms(ctx context.Context) ([]PageForm, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ExtractForms(e.browserCtx)
}

// ---- Wait Primitives ----

// WaitNavigation waits for a navigation event to complete (e.g., after clicking a link).
func (e *Engine) WaitNavigation(ctx context.Context, timeout time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return chromedp.Run(e.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			ch := make(chan struct{}, 1)
			chromedp.ListenTarget(ctx, func(ev interface{}) {
				if _, ok := ev.(*page.EventLoadEventFired); ok {
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			})
			select {
			case <-ch:
				return nil
			case <-waitCtx.Done():
				return fmt.Errorf("wait-navigation: timeout after %s", timeout)
			}
		}),
	)
}

// WaitNetworkIdle waits until no network requests have fired for the given idle duration.
func (e *Engine) WaitNetworkIdle(ctx context.Context, idleDuration, timeout time.Duration) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	deadline := time.After(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	lastActivity := time.Now()

	for {
		select {
		case <-deadline:
			return fmt.Errorf("wait-network-idle: timeout after %s", timeout)
		case <-ticker.C:
			entries := e.network.Entries()
			if len(entries) > 0 {
				// Check if the most recent entry is older than idle duration.
				lastActivity = time.Now()
			}
			if time.Since(lastActivity) >= idleDuration {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// ---- Blocker Stats ----

// BlockerStats returns request blocking statistics. Returns nil if blocker is disabled.
func (e *Engine) BlockerStats() *BlockStats {
	if e.blocker == nil {
		return nil
	}
	stats := e.blocker.Stats()
	return &stats
}

// AddBlockDomain adds a domain to the request block list.
func (e *Engine) AddBlockDomain(domain string) {
	if e.blocker != nil {
		e.blocker.AddDomain(domain)
	}
}

// RemoveBlockDomain removes a domain from the request block list.
func (e *Engine) RemoveBlockDomain(domain string) {
	if e.blocker != nil {
		e.blocker.RemoveDomain(domain)
	}
}

// ---- v0.2.2: Cookies, Storage, Dialogs, Mutations, Frames, Downloads, Overrides, Clipboard ----

// Cookies returns all cookies for the current page.
func (e *Engine) Cookies(ctx context.Context) ([]CookieEntry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return GetCookies(e.browserCtx)
}

// SetCookie sets a single cookie.
func (e *Engine) SetCookie(ctx context.Context, name, value, domain, path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return SetCookie(e.browserCtx, name, value, domain, path)
}

// DeleteCookie deletes cookies by name.
func (e *Engine) DeleteCookie(ctx context.Context, name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return DeleteCookies(e.browserCtx, name)
}

// ClearCookies deletes all cookies.
func (e *Engine) ClearCookies(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClearAllCookies(e.browserCtx)
}

// Storage returns all keys from localStorage or sessionStorage.
func (e *Engine) Storage(ctx context.Context, storageType string) ([]StorageEntry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return GetStorage(e.browserCtx, storageType)
}

// SetStorageItem sets a key-value pair in localStorage or sessionStorage.
func (e *Engine) SetStorageItem(ctx context.Context, storageType, key, value string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return SetStorage(e.browserCtx, storageType, key, value)
}

// ClearStorageAll clears localStorage or sessionStorage.
func (e *Engine) ClearStorageAll(ctx context.Context, storageType string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClearStorage(e.browserCtx, storageType)
}

// DialogEntries returns all captured dialog events.
func (e *Engine) DialogEntries() []DialogEntry {
	if e.dialogs == nil {
		return nil
	}
	return e.dialogs.Entries()
}

// SetDialogMode sets auto-handling behavior: "accept" or "dismiss".
func (e *Engine) SetDialogMode(mode string) {
	if e.dialogs != nil {
		e.dialogs.SetMode(mode)
	}
}

// ClearDialogs removes captured dialog history.
func (e *Engine) ClearDialogs() {
	if e.dialogs != nil {
		e.dialogs.Clear()
	}
}

// StartMutations begins DOM mutation observation.
func (e *Engine) StartMutations(ctx context.Context, selector string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return StartMutationObserver(e.browserCtx, selector)
}

// GetMutations returns captured DOM mutations.
func (e *Engine) GetMutations(ctx context.Context, clear bool) ([]MutationEntry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return GetMutations(e.browserCtx, clear)
}

// StopMutations disconnects the mutation observer.
func (e *Engine) StopMutations(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return StopMutationObserver(e.browserCtx)
}

// FrameList returns all iframes on the current page.
func (e *Engine) FrameList(ctx context.Context) ([]FrameInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ListFrames(e.browserCtx)
}

// FrameContent returns text content from an iframe by index.
func (e *Engine) FrameContent(ctx context.Context, index int) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return GetFrameContent(e.browserCtx, index)
}

// FrameHTML returns HTML from an iframe by index.
func (e *Engine) FrameHTML(ctx context.Context, index int) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return GetFrameHTML(e.browserCtx, index)
}

// DownloadEntries returns all tracked downloads.
func (e *Engine) DownloadEntries() []DownloadEntry {
	if e.downloads == nil {
		return nil
	}
	return e.downloads.Entries()
}

// DownloadScan checks for new downloads.
func (e *Engine) DownloadScan() []DownloadEntry {
	if e.downloads == nil {
		return nil
	}
	return e.downloads.Scan()
}

// DownloadWait waits for a new download to appear (30s default).
func (e *Engine) DownloadWait(timeout time.Duration) (*DownloadEntry, error) {
	if e.downloads == nil {
		return nil, fmt.Errorf("download tracker not initialized")
	}
	return e.downloads.WaitForDownload(timeout)
}

// DownloadDir returns the download directory path.
func (e *Engine) DownloadDir() string {
	if e.downloads == nil {
		return ""
	}
	return e.downloads.DownloadDir()
}

// SetGeo overrides the browser's geolocation.
func (e *Engine) SetGeo(ctx context.Context, lat, lon, accuracy float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return SetGeolocation(e.browserCtx, lat, lon, accuracy)
}

// ClearGeo removes the geolocation override.
func (e *Engine) ClearGeo(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClearGeolocation(e.browserCtx)
}

// SetUA overrides the User-Agent string. Pass a preset name or a custom string.
func (e *Engine) SetUA(ctx context.Context, ua string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	// Check if it's a preset name.
	if preset, ok := UserAgentPresets[ua]; ok {
		ua = preset
	}
	return SetUserAgent(e.browserCtx, ua)
}

// ClipboardRead reads the clipboard content.
func (e *Engine) ClipboardRead(ctx context.Context) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClipboardRead(e.browserCtx)
}

// ClipboardWrite writes text to the clipboard.
func (e *Engine) ClipboardWrite(ctx context.Context, text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClipboardWrite(e.browserCtx, text)
}

// ClipboardClear empties the clipboard.
func (e *Engine) ClipboardClear(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClipboardClear(e.browserCtx)
}

// ---- v0.2.3: Capabilities, History, Recording, Teaching, Styles, Drag ----

// Capabilities returns the full self-describing API.
func (e *Engine) Capabilities() CapabilitiesResponse {
	return GetCapabilities()
}

// SystemPromptText returns the capabilities as a system prompt string.
func (e *Engine) SystemPromptText() string {
	return GetSystemPrompt()
}

// History returns all navigation history entries.
func (e *Engine) History() []HistoryEntry {
	if e.history == nil {
		return nil
	}
	return e.history.Entries()
}

// AddHistory records a URL visit (called by Open).
func (e *Engine) AddHistory(url, title string) {
	if e.history != nil {
		e.history.Add(url, title)
	}
}

// RecordStart begins recording a teaching lesson.
func (e *Engine) RecordStart(title string) {
	if e.recorder != nil {
		e.recorder.Start(title)
	}
}

// RecordStop stops recording and returns the lesson.
func (e *Engine) RecordStop() Lesson {
	if e.recorder == nil {
		return Lesson{}
	}
	return e.recorder.Stop()
}

// RecordSteps returns currently recorded steps without stopping.
func (e *Engine) RecordSteps() []TeachingStep {
	if e.recorder == nil {
		return nil
	}
	return e.recorder.Steps()
}

// RecordNarrate adds a manual narration step.
func (e *Engine) RecordNarrate(text string) {
	if e.recorder == nil {
		return
	}
	url := ""
	if e.history != nil {
		url = e.history.Current()
	}
	e.recorder.Narrate(text, url)
}

// RecordAction records an action (called internally by engine methods).
func (e *Engine) RecordAction(action, target, value string) {
	if e.recorder == nil || !e.recorder.IsRecording() {
		return
	}
	url := ""
	if e.history != nil {
		url = e.history.Current()
	}
	e.recorder.Record(action, target, value, url)
}

// Highlight highlights an element by ref in headed mode.
func (e *Engine) Highlight(ctx context.Context, ref string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	// Convert ref like "e1" to a data-ref selector.
	selector := fmt.Sprintf("[data-itak-ref='%s']", ref)
	return HighlightElement(e.browserCtx, selector)
}

// ShowStep shows a step badge and optional teaching overlay.
func (e *Engine) ShowStep(ctx context.Context, step int, label string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ShowStepBadge(e.browserCtx, step, label)
}

// ClearStep removes the step badge.
func (e *Engine) ClearStep(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClearStepBadge(e.browserCtx)
}

// TeachingOverlay shows a narration bar at the bottom of the page.
func (e *Engine) TeachingOverlay(ctx context.Context, message string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ShowTeachingOverlay(e.browserCtx, message)
}

// ClearOverlay removes the teaching overlay.
func (e *Engine) ClearOverlay(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClearTeachingOverlay(e.browserCtx)
}

// Styles returns computed CSS properties for an element.
func (e *Engine) Styles(ctx context.Context, ref string, properties []string) (map[string]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	selector := fmt.Sprintf("[data-itak-ref='%s']", ref)
	return GetElementStyles(e.browserCtx, selector, properties)
}

// DragDrop performs a drag-and-drop operation by pixel offset.
func (e *Engine) DragDrop(ctx context.Context, selector string, deltaX, deltaY float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	js := fmt.Sprintf(`
		(function() {
			const el = document.querySelector(%q);
			if (!el) return false;
			const rect = el.getBoundingClientRect();
			const startX = rect.left + rect.width / 2;
			const startY = rect.top + rect.height / 2;
			const endX = startX + %f;
			const endY = startY + %f;

			function fire(type, x, y) {
				const evt = new MouseEvent(type, {
					bubbles: true, cancelable: true,
					clientX: x, clientY: y,
					button: 0
				});
				el.dispatchEvent(evt);
			}

			fire('mousedown', startX, startY);
			setTimeout(function() {
				fire('mousemove', (startX + endX) / 2, (startY + endY) / 2);
				setTimeout(function() {
					fire('mousemove', endX, endY);
					setTimeout(function() {
						fire('mouseup', endX, endY);
					}, 50);
				}, 50);
			}, 100);

			return true;
		})()
	`, selector, deltaX, deltaY)

	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil {
		return fmt.Errorf("drag: %w", err)
	}
	if !ok {
		return fmt.Errorf("drag: element not found")
	}
	e.RecordAction("drag", selector, fmt.Sprintf("%g,%g", deltaX, deltaY))
	return nil
}

// CircleRef draws a red circle around an element by ref for teaching.
func (e *Engine) CircleRef(ctx context.Context, ref, label string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	selector := fmt.Sprintf("[data-itak-ref='%s']", ref)
	return CircleElement(e.browserCtx, selector, label)
}

// ArrowRef draws an arrow pointing at an element by ref with a text label.
func (e *Engine) ArrowRef(ctx context.Context, ref, label string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	selector := fmt.Sprintf("[data-itak-ref='%s']", ref)
	return ArrowToElement(e.browserCtx, selector, label)
}

// DrawTextAt places a floating sticky note at a fixed pixel position.
func (e *Engine) DrawTextAt(ctx context.Context, x, y int, text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return DrawText(e.browserCtx, x, y, text)
}

// ClearAnnotations removes all teaching annotations from the page.
func (e *Engine) ClearAnnotations(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClearAnnotations(e.browserCtx)
}

// InjectToolbar injects the annotation toolbar into headed mode.
// Pre-captures a CDP screenshot for pixel-perfect rendering.
func (e *Engine) InjectToolbar(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return InjectToolbarWithScreenshot(e.browserCtx)
}

// RemoveToolbar removes the annotation toolbar.
func (e *Engine) RemoveToolbar(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return RemoveToolbar(e.browserCtx)
}

// GetAnnotatedScreenshot returns the base64 PNG the user drew via the toolbar.
func (e *Engine) GetAnnotatedScreenshot(ctx context.Context) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return GetAnnotatedScreenshot(e.browserCtx)
}

// ClearAnnotatedScreenshot clears the stored annotated screenshot.
func (e *Engine) ClearAnnotatedScreenshot(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return ClearAnnotatedScreenshot(e.browserCtx)
}

// ---- Chrome launch flags ----

// buildFlags assembles Chrome launch flags for headed or headless mode.
func buildFlags(headless, stealth bool, profileDir string, width, height int, proxyServer string) []chromedp.ExecAllocatorOption {
	// chromedp.DefaultExecAllocatorOptions includes Headless by default.
	// For headed mode, we must strip it out so Chrome gets a visible window.
	var base []chromedp.ExecAllocatorOption
	if headless {
		base = chromedp.DefaultExecAllocatorOptions[:]
	} else {
		// What: Build a clean set of allocator options for headed mode.
		// Why:  chromedp.DefaultExecAllocatorOptions includes --no-sandbox
		//       and Headless which cause warning banners in visible Chrome.
		// How:  Specify only the flags we actually need for headed mode.
		base = []chromedp.ExecAllocatorOption{
			chromedp.Flag("disable-background-networking", true),
			chromedp.Flag("disable-client-side-phishing-detection", true),
			chromedp.Flag("disable-default-apps", true),
			chromedp.Flag("disable-hang-monitor", true),
			chromedp.Flag("disable-popup-blocking", false),
			chromedp.Flag("disable-prompt-on-repost", true),
			chromedp.Flag("disable-sync", true),
			chromedp.Flag("metrics-recording-only", true),
			chromedp.Flag("safebrowsing-disable-auto-update", true),
			chromedp.Flag("password-store", "basic"),
		}
	}

	opts := append(base,
		chromedp.UserDataDir(profileDir),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("no-first-run", true),
		// Set viewport size.
		chromedp.WindowSize(width, height),
	)

	if headless {
		opts = append(opts, chromedp.Flag("disable-extensions", true))
	} else {
		// What: Headed-mode specific flags.
		// Why:  Visible browser should look and behave like a real user's Chrome.
		// How:  Suppress automation banner, look like a normal browser.
		opts = append(opts,
			chromedp.Flag("start-maximized", true),
			chromedp.Flag("disable-infobars", true),
			// Force Chrome UI into dark mode.
			chromedp.Flag("force-dark-mode", true),
		)
	}

	if stealth {
		opts = applyStealthFlags(opts)
	}

	// Proxy support.
	if proxyServer != "" {
		opts = append(opts, chromedp.ProxyServer(proxyServer))
	}

	return opts
}

