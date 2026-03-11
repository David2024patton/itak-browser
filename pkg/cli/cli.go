// Package cli provides the gobrowser command-line interface.
//
// What: Cobra-based CLI that proxies every command to the daemon over HTTP.
// Why:  Thin CLI + persistent daemon = fast sequential commands without
//       Chrome cold-start per invocation.
// How:  Each subcommand marshals a cmdRequest, POSTs to the daemon, and
//       pretty-prints or JSON-outputs the cmdResponse.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/David2024patton/iTaKBrowser/pkg/browser"
	"github.com/David2024patton/iTaKBrowser/pkg/daemon"
	"github.com/spf13/cobra"
)

// Config holds global CLI flags.
type Config struct {
	DaemonAddr string // e.g., "127.0.0.1:43721"
	Session    string // Active session ID
	JSONOutput bool   // Output raw JSON instead of human-readable
}

var cfg Config

// daemonPort is the default port matching daemon.DefaultPort.
const daemonPort = 43721

// NewRootCmd builds the CLI command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "gobrowser",
		Short: "iTaK Browser - AI-native browser automation with Snapshot+Refs",
		Long: `gobrowser is the iTaK Browser CLI.
Commands proxy to a persistent daemon that holds the Chrome instance alive
between calls, eliminating cold-start latency for agent workflows.

Example workflow:
  gobrowser session new --headed              # start a visible browser session
  gobrowser open https://example.com          # navigate
  gobrowser snapshot                          # get accessibility tree (~200 tokens)
  gobrowser click e2                          # click the button with ref e2
  gobrowser fill e3 "test@example.com"        # fill the textbox with ref e3
  gobrowser screenshot --annotate             # capture annotated screenshot
  gobrowser debug                             # all-in-one: snapshot+screenshot+console+network+scan
  gobrowser console                           # view page console logs
  gobrowser session close                     # done`,
		SilenceUsage: true,
	}

	// Global flags
	root.PersistentFlags().StringVar(&cfg.DaemonAddr, "daemon", fmt.Sprintf("127.0.0.1:%d", daemonPort), "Daemon address")
	root.PersistentFlags().StringVarP(&cfg.Session, "session", "s", "", "Session ID (use 'session new' to create)")
	root.PersistentFlags().BoolVar(&cfg.JSONOutput, "json", false, "Output raw JSON")

	// Register all subcommand groups.
	root.AddCommand(
		// Session
		newSessionCmd(),
		// Navigation
		newOpenCmd(),
		newSearchCmd(),
		newNavCmd(),
		// Snapshot / Capture
		newSnapshotCmd(),
		newScreenshotCmd(),
		newPDFCmd(),
		// Interaction (v0.1)
		newClickCmd(),
		newFillCmd(),
		newPressCmd(),
		newScrollCmd(),
		newEvalCmd(),
		// Interaction (v0.2)
		newDblClickCmd(),
		newHoverCmd(),
		newSelectCmd(),
		newCheckCmd(),
		newUncheckCmd(),
		newUploadCmd(),
		newMouseCmd(),
		newResizeCmd(),
		// Data Extraction
		newGetCmd(),
		newIsCmd(),
		newWaitCmd(),
		// Debug / Monitoring
		newConsoleCmd(),
		newNetworkCmd(),
		newDebugCmd(),
		// v0.2.1: Tabs, Metrics, Extract, Wait, Webhook, Blocker
		newTabCmd(),
		newMetricsCmd(),
		newLinksCmd(),
		newFormsCmd(),
		newWaitNavCmd(),
		newWaitIdleCmd(),
		newWebhookCmd(),
		newBlockerCmd(),
		// v0.2.2: Cookies, Storage, Dialogs, Mutations, Frames, Downloads, Overrides, Clipboard
		newCookiesCmd(),
		newStorageCmd(),
		newDialogsCmd(),
		newMutationsCmd(),
		newFramesCmd(),
		newDownloadsCmd(),
		newGeoCmd(),
		newUACmd(),
		newClipboardCmd(),
		// v0.2.3: Capabilities, History, Recording, Teaching, Annotations, Styles, Drag
		newCapabilitiesCmd(),
		newHistoryCmd(),
		newRecordCmd(),
		newAnnotateCmd(),
		newHighlightCmd(),
		newStyleCmd(),
		newDragCmd(),
		newToolbarCmd(),
		// v0.3.0: Spotlight, Diff, StorageInspect, A11y, Inspector, Waterfall, Autofill, TabStrip, ScreenRec, Translate
		newSpotlightCmd(),
		newDiffCmd(),
		newStorageInspectCmd(),
		newA11yCmd(),
		newInspectorCmd(),
		newWaterfallCmd(),
		newAutofillCmd(),
		newTabStripCmd(),
		newScreenRecCmd(),
		newTranslateCmd(),
		// Daemon
		newDaemonCmd(),
		newHealthCmd(),
	)

	return root
}

// ---- Session ----

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage browser sessions",
	}

	newCmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new browser session",
		RunE: func(cmd *cobra.Command, args []string) error {
			headed, _ := cmd.Flags().GetBool("headed")
			stealth, _ := cmd.Flags().GetBool("stealth")
			profileDir, _ := cmd.Flags().GetString("profile")
			width, _ := cmd.Flags().GetInt("width")
			height, _ := cmd.Flags().GetInt("height")

			req := map[string]interface{}{
				"headed":        headed,
				"stealth":       stealth,
				"profile_dir":   profileDir,
				"window_width":  width,
				"window_height": height,
			}
			ensureDaemon(cfg.DaemonAddr)

			resp, err := post(cfg.DaemonAddr, "/session/new", req)
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
	newCmd.Flags().Bool("headed", false, "Launch visible (headed) Chrome window")
	newCmd.Flags().Bool("stealth", false, "Enable anti-detection stealth mode")
	newCmd.Flags().String("profile", "", "Custom profile directory path")
	newCmd.Flags().Int("width", 1920, "Viewport width in pixels")
	newCmd.Flags().Int("height", 1080, "Viewport height in pixels")

	closeCmd := &cobra.Command{
		Use:   "close",
		Short: "Close the active session",
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/session/close", cfg.Session, nil)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all active sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := get(cfg.DaemonAddr, "/sessions")
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}

	saveCmd := &cobra.Command{
		Use:   "save",
		Short: "Save session cookies to encrypted disk",
		RunE: func(cmd *cobra.Command, args []string) error {
			phrase, _ := cmd.Flags().GetString("passphrase")
			return sendSession(cfg.DaemonAddr, "/session/save", cfg.Session, map[string]string{"passphrase": phrase})
		},
	}
	saveCmd.Flags().String("passphrase", "itak-default", "Encryption passphrase")

	restoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore session cookies from encrypted disk",
		RunE: func(cmd *cobra.Command, args []string) error {
			phrase, _ := cmd.Flags().GetString("passphrase")
			return sendSession(cfg.DaemonAddr, "/session/restore", cfg.Session, map[string]string{"passphrase": phrase})
		},
	}
	restoreCmd.Flags().String("passphrase", "itak-default", "Encryption passphrase")

	cmd.AddCommand(newCmd, closeCmd, listCmd, saveCmd, restoreCmd)
	return cmd
}

// ---- Navigation ----

func newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open <url>",
		Short: "Navigate to a URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := post(cfg.DaemonAddr, "/open", map[string]string{
				"session": cfg.Session, "url": args[0],
			})
			if err != nil {
				return err
			}
			// Check for poisoning warning and print in red.
			checkPoisonWarning(resp)
			if cfg.JSONOutput {
				return printResp(resp)
			}
			return nil
		},
	}
}

func newSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query...>",
		Short: "Search via self-hosted SearXNG (bypasses bot detection, aggregates 10+ engines)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			resp, err := post(cfg.DaemonAddr, "/search", map[string]string{
				"session": cfg.Session, "text": query,
			})
			if err != nil {
				return err
			}
			if cfg.JSONOutput {
				return printResp(resp)
			}
			return nil
		},
	}
}

func newNavCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nav",
		Short: "Navigation commands (back, forward, reload)",
	}
	cmd.AddCommand(
		&cobra.Command{Use: "back", Short: "Go back", RunE: func(_ *cobra.Command, _ []string) error {
			return sendSession(cfg.DaemonAddr, "/back", cfg.Session, nil)
		}},
		&cobra.Command{Use: "forward", Short: "Go forward", RunE: func(_ *cobra.Command, _ []string) error {
			return sendSession(cfg.DaemonAddr, "/forward", cfg.Session, nil)
		}},
		&cobra.Command{Use: "reload", Short: "Reload page", RunE: func(_ *cobra.Command, _ []string) error {
			return sendSession(cfg.DaemonAddr, "/reload", cfg.Session, nil)
		}},
	)
	return cmd
}

// ---- Snapshot / Capture ----

func newSnapshotCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "snapshot",
		Short: "Get the current page as a compact accessibility tree",
		Long:  "Outputs the Snapshot+Refs tree (~200 tokens vs 10,000 for raw HTML).",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := post(cfg.DaemonAddr, "/snapshot", map[string]string{"session": cfg.Session})
			if err != nil {
				return err
			}
			if cfg.JSONOutput {
				return printResp(resp)
			}
			if snap, ok := resp["data"].(map[string]interface{}); ok {
				fmt.Printf("URL:   %s\n", snap["url"])
				fmt.Printf("Title: %s\n", snap["title"])
				fmt.Println()
				fmt.Println(snap["tree"])
			}
			return nil
		},
	}
}

func newScreenshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "screenshot",
		Short: "Capture the current viewport as PNG",
		RunE: func(cmd *cobra.Command, args []string) error {
			annotate, _ := cmd.Flags().GetBool("annotate")
			resp, err := post(cfg.DaemonAddr, "/screenshot", map[string]interface{}{
				"session":  cfg.Session,
				"annotate": annotate,
			})
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
	cmd.Flags().Bool("annotate", false, "Overlay numbered element labels for vision models")
	return cmd
}

func newPDFCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pdf <output-path>",
		Short: "Capture the current page as a PDF",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/pdf", cfg.Session, map[string]string{"path": args[0]})
		},
	}
}

// ---- Interaction (v0.1) ----

func newClickCmd() *cobra.Command {
	return &cobra.Command{
		Use: "click <ref>", Short: "Click an element by ref ID", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/click", cfg.Session, map[string]string{"ref": args[0]})
		},
	}
}

func newFillCmd() *cobra.Command {
	return &cobra.Command{
		Use: "fill <ref> <text>", Short: "Fill a text input by ref ID", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/fill", cfg.Session, map[string]string{"ref": args[0], "text": args[1]})
		},
	}
}

func newPressCmd() *cobra.Command {
	return &cobra.Command{
		Use: "press <key>", Short: "Send a key press (Enter, Tab, Escape)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/press", cfg.Session, map[string]string{"text": args[0]})
		},
	}
}

func newScrollCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "scroll", Short: "Scroll the page",
		RunE: func(cmd *cobra.Command, args []string) error {
			dy, _ := cmd.Flags().GetFloat64("y")
			dx, _ := cmd.Flags().GetFloat64("x")
			resp, err := post(cfg.DaemonAddr, "/scroll", map[string]interface{}{
				"session": cfg.Session, "delta_x": dx, "delta_y": dy,
			})
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
	cmd.Flags().Float64("y", 300, "Vertical scroll delta (positive = down)")
	cmd.Flags().Float64("x", 0, "Horizontal scroll delta")
	return cmd
}

func newEvalCmd() *cobra.Command {
	return &cobra.Command{
		Use: "eval <js>", Short: "Execute JavaScript in the page context", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := post(cfg.DaemonAddr, "/eval", map[string]string{"session": cfg.Session, "js": args[0]})
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
}

// ---- Interaction (v0.2 - headed mode expansion) ----

func newDblClickCmd() *cobra.Command {
	return &cobra.Command{
		Use: "dblclick <ref>", Short: "Double-click an element by ref ID", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/dblclick", cfg.Session, map[string]string{"ref": args[0]})
		},
	}
}

func newHoverCmd() *cobra.Command {
	return &cobra.Command{
		Use: "hover <ref>", Short: "Hover over an element by ref ID", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/hover", cfg.Session, map[string]string{"ref": args[0]})
		},
	}
}

func newSelectCmd() *cobra.Command {
	return &cobra.Command{
		Use: "select <ref> <value>", Short: "Select a dropdown option by ref ID", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := post(cfg.DaemonAddr, "/select", map[string]interface{}{
				"session": cfg.Session, "ref": args[0], "values": []string{args[1]},
			})
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
}

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use: "check <ref>", Short: "Check a checkbox by ref ID", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/check", cfg.Session, map[string]string{"ref": args[0]})
		},
	}
}

func newUncheckCmd() *cobra.Command {
	return &cobra.Command{
		Use: "uncheck <ref>", Short: "Uncheck a checkbox by ref ID", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/uncheck", cfg.Session, map[string]string{"ref": args[0]})
		},
	}
}

func newUploadCmd() *cobra.Command {
	return &cobra.Command{
		Use: "upload <ref> <file-path>", Short: "Upload a file to an input[type=file]", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/upload", cfg.Session, map[string]string{"ref": args[0], "path": args[1]})
		},
	}
}

func newMouseCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "mouse", Short: "Pixel-precise mouse control"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "move <x> <y>", Short: "Move mouse to pixel coordinates", Args: cobra.ExactArgs(2),
			RunE: func(_ *cobra.Command, args []string) error {
				x, _ := strconv.ParseFloat(args[0], 64)
				y, _ := strconv.ParseFloat(args[1], 64)
				resp, err := post(cfg.DaemonAddr, "/mouse-move", map[string]interface{}{
					"session": cfg.Session, "x": x, "y": y,
				})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "click <x> <y> [button]", Short: "Click at pixel coordinates", Args: cobra.RangeArgs(2, 3),
			RunE: func(_ *cobra.Command, args []string) error {
				x, _ := strconv.ParseFloat(args[0], 64)
				y, _ := strconv.ParseFloat(args[1], 64)
				button := "left"
				if len(args) > 2 {
					button = args[2]
				}
				resp, err := post(cfg.DaemonAddr, "/mouse-click", map[string]interface{}{
					"session": cfg.Session, "x": x, "y": y, "button": button,
				})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
	)
	return cmd
}

func newResizeCmd() *cobra.Command {
	return &cobra.Command{
		Use: "resize <width> <height>", Short: "Resize the browser viewport", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			w, _ := strconv.Atoi(args[0])
			h, _ := strconv.Atoi(args[1])
			resp, err := post(cfg.DaemonAddr, "/resize", map[string]interface{}{
				"session": cfg.Session, "width": w, "height": h,
			})
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
}

// ---- Data Extraction ----

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "get", Short: "Extract data from the page"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "text <ref>", Short: "Get visible text of an element", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				resp, err := post(cfg.DaemonAddr, "/get-text", map[string]string{"session": cfg.Session, "ref": args[0]})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "html <ref>", Short: "Get outer HTML of an element", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				resp, err := post(cfg.DaemonAddr, "/get-html", map[string]string{"session": cfg.Session, "ref": args[0]})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "title", Short: "Get the current page title",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/get-title", map[string]string{"session": cfg.Session})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "url", Short: "Get the current page URL",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/get-url", map[string]string{"session": cfg.Session})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "value <ref>", Short: "Get the .value of a form element", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				resp, err := post(cfg.DaemonAddr, "/get-value", map[string]string{"session": cfg.Session, "ref": args[0]})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "attr <ref> <name>", Short: "Get any HTML attribute", Args: cobra.ExactArgs(2),
			RunE: func(_ *cobra.Command, args []string) error {
				resp, err := post(cfg.DaemonAddr, "/get-attr", map[string]string{"session": cfg.Session, "ref": args[0], "attr": args[1]})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "box <ref>", Short: "Get bounding box {x, y, width, height}", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				resp, err := post(cfg.DaemonAddr, "/get-box", map[string]string{"session": cfg.Session, "ref": args[0]})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
	)
	return cmd
}

func newIsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "is", Short: "Element state checks"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "visible <ref>", Short: "Check if element is visible", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				resp, err := post(cfg.DaemonAddr, "/is-visible", map[string]string{"session": cfg.Session, "ref": args[0]})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "enabled <ref>", Short: "Check if element is enabled", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				resp, err := post(cfg.DaemonAddr, "/is-enabled", map[string]string{"session": cfg.Session, "ref": args[0]})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "checked <ref>", Short: "Check if checkbox/radio is checked", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				resp, err := post(cfg.DaemonAddr, "/is-checked", map[string]string{"session": cfg.Session, "ref": args[0]})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
	)
	return cmd
}

func newWaitCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "wait", Short: "Wait for page conditions"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "visible <selector>", Short: "Wait until a CSS selector is visible", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/wait-visible", cfg.Session, map[string]string{"selector": args[0]})
			},
		},
	)
	return cmd
}

// ---- Debug / Monitoring ----

func newConsoleCmd() *cobra.Command {
	return &cobra.Command{
		Use: "console", Short: "Show captured console log/warn/error messages",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := post(cfg.DaemonAddr, "/console", map[string]string{"session": cfg.Session})
			if err != nil {
				return err
			}
			if cfg.JSONOutput {
				return printResp(resp)
			}
			// Human-readable console output.
			if data, ok := resp["data"].([]interface{}); ok {
				for _, entry := range data {
					if m, ok := entry.(map[string]interface{}); ok {
						fmt.Printf("[%s] %s\n", m["level"], m["text"])
					}
				}
				if len(data) == 0 {
					fmt.Println("(no console messages captured)")
				}
			}
			return nil
		},
	}
}

func newNetworkCmd() *cobra.Command {
	return &cobra.Command{
		Use: "network", Short: "Show captured network requests/responses",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := post(cfg.DaemonAddr, "/network", map[string]string{"session": cfg.Session})
			if err != nil {
				return err
			}
			if cfg.JSONOutput {
				return printResp(resp)
			}
			if data, ok := resp["data"].([]interface{}); ok {
				for _, entry := range data {
					if m, ok := entry.(map[string]interface{}); ok {
						status := ""
						if s, ok := m["status"].(float64); ok && s > 0 {
							status = fmt.Sprintf(" -> %d", int(s))
						}
						fmt.Printf("%s %s%s\n", m["method"], m["url"], status)
					}
				}
				if len(data) == 0 {
					fmt.Println("(no network requests captured)")
				}
			}
			return nil
		},
	}
}

func newDebugCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "debug",
		Short: "All-in-one debug dump: snapshot + screenshot + console + network + content scan",
		Long: `Produces a single JSON response with everything an agent needs for debugging:
  - Current URL and title
  - Accessibility tree (Snapshot+Refs)
  - Screenshot path
  - Console log messages
  - Network request log
  - ContentGuard scan report (AI recommendation poisoning detection)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := post(cfg.DaemonAddr, "/debug", map[string]string{"session": cfg.Session})
			if err != nil {
				return err
			}
			if cfg.JSONOutput {
				return printResp(resp)
			}
			// Human-readable debug summary.
			if data, ok := resp["data"].(map[string]interface{}); ok {
				fmt.Printf("URL:        %s\n", data["url"])
				fmt.Printf("Title:      %s\n", data["title"])
				fmt.Printf("Screenshot: %s\n", data["screenshot_path"])
				if scan, ok := data["scan_report"].(map[string]interface{}); ok {
					clean, _ := scan["clean"].(bool)
					if clean {
						fmt.Println("Security:   CLEAN (no poisoning detected)")
					} else {
						fmt.Println("Security:   WARNING - potential AI recommendation poisoning detected")
						if warnings, ok := scan["warnings"].([]interface{}); ok {
							for _, w := range warnings {
								fmt.Printf("  ! %s\n", w)
							}
						}
					}
				}
				fmt.Println()
				fmt.Println(data["tree"])
			}
			return nil
		},
	}
}

// ---- Daemon Management ----

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "daemon", Short: "Manage the browser daemon"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "start", Short: "Start the browser daemon in the foreground",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Printf("Starting iTaK Browser daemon on %s\n", cfg.DaemonAddr)
				return startDaemon(cfg.DaemonAddr)
			},
		},
		&cobra.Command{
			Use: "stop", Short: "Stop the running daemon",
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println("Daemon stop not yet implemented (kill the process directly for now)")
				return nil
			},
		},
		&cobra.Command{
			Use: "status", Short: "Check if the daemon is running",
			RunE: func(cmd *cobra.Command, args []string) error {
				resp, err := get(cfg.DaemonAddr, "/health")
				if err != nil {
					fmt.Println("Daemon is NOT running:", err)
					return nil
				}
				return printResp(resp)
			},
		},
	)
	return cmd
}

func newHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use: "health", Short: "Check daemon health",
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := get(cfg.DaemonAddr, "/health")
			if err != nil {
				return fmt.Errorf("daemon is not running: %w", err)
			}
			return printResp(resp)
		},
	}
}

// ---- HTTP helpers ----

var httpClient = &http.Client{Timeout: 90 * time.Second}

func post(addr, path string, body interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Post("http://"+addr+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("daemon unreachable at %s: %w", addr, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("bad daemon response: %s", string(raw))
	}

	if ok, _ := result["ok"].(bool); !ok {
		if errMsg, _ := result["error"].(string); errMsg != "" {
			return nil, fmt.Errorf("browser error: %s", errMsg)
		}
	}

	return result, nil
}

func get(addr, path string) (map[string]interface{}, error) {
	resp, err := httpClient.Get("http://" + addr + path)
	if err != nil {
		return nil, fmt.Errorf("daemon unreachable at %s: %w", addr, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	return result, json.Unmarshal(raw, &result)
}

func sendSession(addr, path, sessionID string, extra map[string]string) error {
	body := map[string]interface{}{"session": sessionID}
	for k, v := range extra {
		body[k] = v
	}
	resp, err := post(addr, path, body)
	if err != nil {
		return err
	}
	return printResp(resp)
}

func printResp(resp map[string]interface{}) error {
	if cfg.JSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	}

	if sid, _ := resp["session"].(string); sid != "" {
		fmt.Printf("session: %s\n", sid)
	}
	if data := resp["data"]; data != nil {
		switch v := data.(type) {
		case string:
			fmt.Println(v)
		case map[string]interface{}:
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(v)
		case []interface{}:
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(v)
		default:
			fmt.Printf("%v\n", v)
		}
	}
	return nil
}

func ensureDaemon(addr string) {
	_, err := get(addr, "/health")
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "Daemon not running. Start it with: gobrowser daemon start\n")
}

func startDaemon(addr string) error {
	// Parse port from addr (e.g., "127.0.0.1:43721" -> 43721).
	port := daemonPort
	if _, p, err := net.SplitHostPort(addr); err == nil {
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}

	logger := slog.Default()
	d := daemon.New(port, logger)

	// Graceful shutdown on Ctrl+C.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down daemon...")
		os.Exit(0)
	}()

	fmt.Printf("iTaK Browser v%s daemon listening on :%d\n", browser.Version, port)
	return d.Start()
}

// ---- ANSI Red Poisoning Warning ----

// ANSI color codes for terminal output.
const (
	ansiRed    = "\033[1;31m"
	ansiYellow = "\033[1;33m"
	ansiReset  = "\033[0m"
)

// checkPoisonWarning checks a daemon response for scan_result and prints
// a red warning banner in the CLI if poisoning was detected.
func checkPoisonWarning(resp map[string]interface{}) {
	data, _ := resp["data"].(map[string]interface{})
	if data == nil {
		return
	}
	scan, _ := data["scan_result"].(map[string]interface{})
	if scan == nil {
		return
	}
	clean, _ := scan["clean"].(bool)
	if clean {
		return
	}

	// Print red warning banner.
	fmt.Fprintf(os.Stderr, "\n%s========================================%s\n", ansiRed, ansiReset)
	fmt.Fprintf(os.Stderr, "%s  ⚠️  CONTENTGUARD: POISONING DETECTED  %s\n", ansiRed, ansiReset)
	fmt.Fprintf(os.Stderr, "%s========================================%s\n", ansiRed, ansiReset)

	if warnings, ok := scan["warnings"].([]interface{}); ok {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "%s  ! %s%s\n", ansiYellow, w, ansiReset)
		}
	}

	if msg, ok := data["poisoning_warning"].(string); ok {
		fmt.Fprintf(os.Stderr, "%s  %s%s\n", ansiRed, msg, ansiReset)
	}
	fmt.Fprintln(os.Stderr)
}

// ---- v0.2.1 CLI Commands ----

func newTabCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tab", Short: "Manage browser tabs"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "new [url]", Short: "Open a new tab", Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				url := ""
				if len(args) > 0 {
					url = args[0]
				}
				resp, err := post(cfg.DaemonAddr, "/tab/new", map[string]string{
					"session": cfg.Session, "url": url,
				})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "switch <tab-id>", Short: "Switch to a tab", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/tab/switch", cfg.Session, map[string]string{"ref": args[0]})
			},
		},
		&cobra.Command{
			Use: "close <tab-id>", Short: "Close a tab", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/tab/close", cfg.Session, map[string]string{"ref": args[0]})
			},
		},
		&cobra.Command{
			Use: "list", Short: "List all open tabs",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/tab/list", map[string]string{"session": cfg.Session})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
	)
	return cmd
}

func newMetricsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "metrics", Short: "Get page performance metrics (DOM nodes, load time, memory)",
		RunE: func(_ *cobra.Command, _ []string) error {
			resp, err := post(cfg.DaemonAddr, "/metrics", map[string]string{"session": cfg.Session})
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
}

func newLinksCmd() *cobra.Command {
	return &cobra.Command{
		Use: "links", Short: "Extract all hyperlinks from the current page",
		RunE: func(_ *cobra.Command, _ []string) error {
			resp, err := post(cfg.DaemonAddr, "/get-links", map[string]string{"session": cfg.Session})
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
}

func newFormsCmd() *cobra.Command {
	return &cobra.Command{
		Use: "forms", Short: "Extract all forms and fields from the current page",
		RunE: func(_ *cobra.Command, _ []string) error {
			resp, err := post(cfg.DaemonAddr, "/get-forms", map[string]string{"session": cfg.Session})
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
}

func newWaitNavCmd() *cobra.Command {
	return &cobra.Command{
		Use: "wait-nav", Short: "Wait for a navigation event to complete (30s timeout)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return sendSession(cfg.DaemonAddr, "/wait-nav", cfg.Session, nil)
		},
	}
}

func newWaitIdleCmd() *cobra.Command {
	return &cobra.Command{
		Use: "wait-idle", Short: "Wait until no network activity for 2 seconds (30s timeout)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return sendSession(cfg.DaemonAddr, "/wait-idle", cfg.Session, nil)
		},
	}
}

func newWebhookCmd() *cobra.Command {
	return &cobra.Command{
		Use: "webhook <url>", Short: "Set threat detection webhook URL for real-time notifications", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			resp, err := post(cfg.DaemonAddr, "/webhook", map[string]string{"url": args[0]})
			if err != nil {
				return err
			}
			return printResp(resp)
		},
	}
}

func newBlockerCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "blocker", Short: "Manage request blocking (ads, trackers)"}
	cmd.AddCommand(
		&cobra.Command{
			Use: "stats", Short: "Show blocking statistics",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/blocker/stats", map[string]string{"session": cfg.Session})
				if err != nil {
					return err
				}
				return printResp(resp)
			},
		},
		&cobra.Command{
			Use: "add <domain>", Short: "Add a domain to the block list", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/blocker/add", cfg.Session, map[string]string{"url": args[0]})
			},
		},
		&cobra.Command{
			Use: "remove <domain>", Short: "Remove a domain from the block list", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/blocker/remove", cfg.Session, map[string]string{"url": args[0]})
			},
		},
	)
	return cmd
}

// ---- v0.2.2 CLI Commands ----

func newCookiesCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "cookies", Short: "Manage browser cookies"}
	cmd.AddCommand(
		&cobra.Command{Use: "get", Short: "List all cookies",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/cookies", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "set <name> <value> [domain]", Short: "Set a cookie", Args: cobra.RangeArgs(2, 3),
			RunE: func(_ *cobra.Command, args []string) error {
				domain := ""
				if len(args) > 2 { domain = args[2] }
				return sendSession(cfg.DaemonAddr, "/cookies/set", cfg.Session, map[string]string{
					"key": args[0], "value": args[1], "domain": domain,
				})
			}},
		&cobra.Command{Use: "delete <name>", Short: "Delete a cookie", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/cookies/delete", cfg.Session, map[string]string{"key": args[0]})
			}},
		&cobra.Command{Use: "clear", Short: "Delete all cookies",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/cookies/clear", cfg.Session, nil)
			}},
	)
	return cmd
}

func newStorageCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "storage", Short: "Manage localStorage/sessionStorage"}
	cmd.AddCommand(
		&cobra.Command{Use: "get [local|session]", Short: "List all storage entries", Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				st := "local"
				if len(args) > 0 { st = args[0] }
				resp, err := post(cfg.DaemonAddr, "/storage", map[string]string{
					"session": cfg.Session, "storage_type": st,
				})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "set <key> <value> [local|session]", Short: "Set a storage entry", Args: cobra.RangeArgs(2, 3),
			RunE: func(_ *cobra.Command, args []string) error {
				st := "local"
				if len(args) > 2 { st = args[2] }
				return sendSession(cfg.DaemonAddr, "/storage/set", cfg.Session, map[string]string{
					"key": args[0], "value": args[1], "storage_type": st,
				})
			}},
		&cobra.Command{Use: "clear [local|session]", Short: "Clear all storage entries", Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				st := "local"
				if len(args) > 0 { st = args[0] }
				return sendSession(cfg.DaemonAddr, "/storage/clear", cfg.Session, map[string]string{"storage_type": st})
			}},
	)
	return cmd
}

func newDialogsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "dialogs", Short: "Manage JS dialog auto-handling (alert/confirm/prompt)"}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "Show captured dialog events",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/dialogs", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "mode <accept|dismiss>", Short: "Set auto-handling behavior", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/dialogs/mode", cfg.Session, map[string]string{"mode": args[0]})
			}},
	)
	return cmd
}

func newMutationsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "mutations", Short: "DOM mutation observation"}
	cmd.AddCommand(
		&cobra.Command{Use: "start [selector]", Short: "Begin watching DOM changes", Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				sel := ""
				if len(args) > 0 { sel = args[0] }
				return sendSession(cfg.DaemonAddr, "/mutations/start", cfg.Session, map[string]string{"selector": sel})
			}},
		&cobra.Command{Use: "get", Short: "Retrieve captured DOM mutations",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/mutations", map[string]string{
					"session": cfg.Session,
				})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "stop", Short: "Stop watching DOM changes",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/mutations/stop", cfg.Session, nil)
			}},
	)
	return cmd
}

func newFramesCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "frames", Short: "Manage iframes"}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List all iframes on the page",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/frames", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "content <index>", Short: "Get text content from an iframe", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				resp, err := post(cfg.DaemonAddr, "/frames/content", map[string]string{
					"session": cfg.Session, "index": args[0],
				})
				if err != nil { return err }
				return printResp(resp)
			}},
	)
	return cmd
}

func newDownloadsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "downloads", Short: "Track file downloads"}
	cmd.AddCommand(
		&cobra.Command{Use: "list", Short: "List tracked downloads",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/downloads", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "scan", Short: "Scan for new downloads",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/downloads/scan", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
	)
	return cmd
}

func newGeoCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "geo", Short: "Spoof browser geolocation"}
	cmd.AddCommand(
		&cobra.Command{Use: "set <lat> <lon>", Short: "Set GPS coordinates", Args: cobra.ExactArgs(2),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/geo/set", cfg.Session, map[string]string{
					"lat": args[0], "lon": args[1],
				})
			}},
		&cobra.Command{Use: "clear", Short: "Remove geolocation override",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/geo/clear", cfg.Session, nil)
			}},
	)
	return cmd
}

func newUACmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ua <preset-or-custom-string>",
		Short: "Set User-Agent (presets: chrome-win, chrome-mac, firefox-win, safari-mac, iphone, android, googlebot)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/ua/set", cfg.Session, map[string]string{"value": args[0]})
		},
	}
}

func newClipboardCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "clipboard", Short: "Read/write browser clipboard"}
	cmd.AddCommand(
		&cobra.Command{Use: "read", Short: "Read clipboard text",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/clipboard", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "write <text>", Short: "Write text to clipboard", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/clipboard/write", cfg.Session, map[string]string{"text": args[0]})
			}},
	)
	return cmd
}

// ---- v0.2.3 CLI Commands ----

func newCapabilitiesCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "capabilities", Short: "List all available browser commands (self-describing API)"}
	cmd.AddCommand(
		&cobra.Command{Use: "json", Short: "Get capabilities as JSON",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := http.Get("http://" + cfg.DaemonAddr + "/capabilities")
				if err != nil { return err }
				defer resp.Body.Close()
				var buf bytes.Buffer
				buf.ReadFrom(resp.Body)
				fmt.Println(buf.String())
				return nil
			}},
		&cobra.Command{Use: "prompt", Short: "Get capabilities as an AI system prompt",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := http.Get("http://" + cfg.DaemonAddr + "/system-prompt")
				if err != nil { return err }
				defer resp.Body.Close()
				var buf bytes.Buffer
				buf.ReadFrom(resp.Body)
				fmt.Println(buf.String())
				return nil
			}},
	)
	return cmd
}

func newHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show navigation history (URLs visited in this session)",
		RunE: func(_ *cobra.Command, _ []string) error {
			resp, err := post(cfg.DaemonAddr, "/history", map[string]string{"session": cfg.Session})
			if err != nil { return err }
			return printResp(resp)
		},
	}
}

func newRecordCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "record", Short: "Teaching lesson recorder"}
	cmd.AddCommand(
		&cobra.Command{Use: "start [title]", Short: "Begin recording a lesson", Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				title := "Untitled Lesson"
				if len(args) > 0 { title = args[0] }
				return sendSession(cfg.DaemonAddr, "/record/start", cfg.Session, map[string]string{"title": title})
			}},
		&cobra.Command{Use: "stop", Short: "Stop recording and export the lesson",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/record/stop", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "steps", Short: "Show current recorded steps",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/record/steps", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "narrate <text>", Short: "Add a narration/explanation step", Args: cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/record/narrate", cfg.Session, map[string]string{"text": args[0]})
			}},
	)
	return cmd
}

func newAnnotateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "annotate", Short: "Draw annotations on screen (teaching mode)"}
	cmd.AddCommand(
		&cobra.Command{Use: "circle <ref> [label]", Short: "Draw a red circle around an element", Args: cobra.RangeArgs(1, 2),
			RunE: func(_ *cobra.Command, args []string) error {
				label := ""
				if len(args) > 1 { label = args[1] }
				return sendSession(cfg.DaemonAddr, "/annotate/circle", cfg.Session, map[string]string{
					"ref": args[0], "label": label,
				})
			}},
		&cobra.Command{Use: "arrow <ref> [label]", Short: "Draw an arrow pointing at an element", Args: cobra.RangeArgs(1, 2),
			RunE: func(_ *cobra.Command, args []string) error {
				label := ""
				if len(args) > 1 { label = args[1] }
				return sendSession(cfg.DaemonAddr, "/annotate/arrow", cfg.Session, map[string]string{
					"ref": args[0], "label": label,
				})
			}},
		&cobra.Command{Use: "text <x> <y> <text>", Short: "Place a sticky note on screen", Args: cobra.ExactArgs(3),
			RunE: func(_ *cobra.Command, args []string) error {
				return sendSession(cfg.DaemonAddr, "/annotate/text", cfg.Session, map[string]string{
					"x": args[0], "y": args[1], "text": args[2],
				})
			}},
		&cobra.Command{Use: "clear", Short: "Remove all annotations from screen",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/annotate/clear", cfg.Session, nil)
			}},
	)
	return cmd
}

func newHighlightCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "highlight <ref>",
		Short: "Highlight an element with a blue glow (3s, auto-scroll)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/highlight", cfg.Session, map[string]string{"ref": args[0]})
		},
	}
}

func newStyleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "style <ref>",
		Short: "Get computed CSS styles for an element",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			resp, err := post(cfg.DaemonAddr, "/style", map[string]string{
				"session": cfg.Session, "ref": args[0],
			})
			if err != nil { return err }
			return printResp(resp)
		},
	}
}

func newDragCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "drag <ref> <delta_x> <delta_y>",
		Short: "Drag an element by pixel offset",
		Args:  cobra.ExactArgs(3),
		RunE: func(_ *cobra.Command, args []string) error {
			return sendSession(cfg.DaemonAddr, "/drag", cfg.Session, map[string]string{
				"ref": args[0], "delta_x": args[1], "delta_y": args[2],
			})
		},
	}
}

func newToolbarCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "toolbar", Short: "Annotation toolbar for headed mode (user markup)"}
	cmd.AddCommand(
		&cobra.Command{Use: "show", Short: "Inject the annotation toolbar into the browser",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/toolbar", cfg.Session, nil)
			}},
		&cobra.Command{Use: "hide", Short: "Remove the annotation toolbar",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/toolbar/remove", cfg.Session, nil)
			}},
		&cobra.Command{Use: "get-markup", Short: "Get the annotated screenshot from the user",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/toolbar/markup", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
	)
	return cmd
}
