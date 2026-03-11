// Package cli - v0.3.0 CLI commands.
//
// What: Cobra commands for all v0.3.0 features: Spotlight, Diff, Storage
//       Inspector, A11y Audit, Element Inspector, Waterfall, Autofill,
//       TabStrip, Screen Recording, and Page Translation.
// Why:  Keeps v0.3.0 CLI commands separate from the growing cli.go.
// How:  Each command follows the sendSession/post pattern to proxy
//       requests to the daemon HTTP API.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ---- Spotlight ----

func newSpotlightCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "spotlight", Short: "AI spotlight mode - dims page except target element"}
	cmd.AddCommand(
		&cobra.Command{Use: "show [selector] [label]", Short: "Spotlight an element",
			Args: cobra.RangeArgs(1, 2),
			RunE: func(_ *cobra.Command, args []string) error {
				payload := map[string]string{"session": cfg.Session, "selector": args[0]}
				if len(args) > 1 { payload["label"] = args[1] }
				return sendSession(cfg.DaemonAddr, "/spotlight", cfg.Session, payload)
			}},
		&cobra.Command{Use: "clear", Short: "Remove spotlight overlay",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/spotlight/clear", cfg.Session, nil)
			}},
	)
	return cmd
}

// ---- Visual Diff ----

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "diff", Short: "Visual diff - snapshot before/after and overlay changes"}
	cmd.AddCommand(
		&cobra.Command{Use: "snapshot [name]", Short: "Capture a 'before' snapshot",
			Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				name := "default"
				if len(args) > 0 { name = args[0] }
				return sendSession(cfg.DaemonAddr, "/diff/snapshot", cfg.Session,
					map[string]string{"session": cfg.Session, "name": name})
			}},
		&cobra.Command{Use: "compare [name]", Short: "Compare current state against snapshot",
			Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				name := "default"
				if len(args) > 0 { name = args[0] }
				return sendSession(cfg.DaemonAddr, "/diff/compare", cfg.Session,
					map[string]string{"session": cfg.Session, "name": name})
			}},
		&cobra.Command{Use: "clear", Short: "Remove diff overlay",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/diff/clear", cfg.Session, nil)
			}},
	)
	return cmd
}

// ---- Storage Inspector ----

func newStorageInspectCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "storage-inspect", Short: "Cookie/localStorage/sessionStorage visual inspector"}
	cmd.AddCommand(
		&cobra.Command{Use: "show", Short: "Open storage inspector panel",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/storage-inspect", cfg.Session, nil)
			}},
		&cobra.Command{Use: "close", Short: "Close storage inspector panel",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/storage-inspect/close", cfg.Session, nil)
			}},
	)
	return cmd
}

// ---- Accessibility Audit ----

func newA11yCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "a11y", Short: "Accessibility audit overlay with colour-coded badges"}
	cmd.AddCommand(
		&cobra.Command{Use: "audit", Short: "Run accessibility audit and show badges",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/a11y", cfg.Session, nil)
			}},
		&cobra.Command{Use: "clear", Short: "Remove audit badges",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/a11y/clear", cfg.Session, nil)
			}},
	)
	return cmd
}

// ---- Element Inspector ----

func newInspectorCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "inspector", Short: "Element inspector panel - DOM, CSS, box model"}
	cmd.AddCommand(
		&cobra.Command{Use: "open", Short: "Open inspector and enable click-to-inspect",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/inspector", cfg.Session, nil)
			}},
		&cobra.Command{Use: "close", Short: "Close inspector panel",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/inspector/close", cfg.Session, nil)
			}},
	)
	return cmd
}

// ---- Performance Waterfall ----

func newWaterfallCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "waterfall", Short: "Network performance waterfall chart"}
	cmd.AddCommand(
		&cobra.Command{Use: "show", Short: "Open waterfall panel",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/waterfall", cfg.Session, nil)
			}},
		&cobra.Command{Use: "close", Short: "Close waterfall panel",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/waterfall/close", cfg.Session, nil)
			}},
	)
	return cmd
}

// ---- Form Autofill ----

func newAutofillCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "autofill", Short: "Save and replay form fill profiles"}
	cmd.AddCommand(
		&cobra.Command{Use: "save [name]", Short: "Save current form state as a profile",
			Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				name := "default"
				if len(args) > 0 { name = args[0] }
				return sendSession(cfg.DaemonAddr, "/autofill/save", cfg.Session,
					map[string]string{"session": cfg.Session, "name": name})
			}},
		&cobra.Command{Use: "load [name]", Short: "Restore form fields from a profile",
			Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				name := "default"
				if len(args) > 0 { name = args[0] }
				return sendSession(cfg.DaemonAddr, "/autofill/load", cfg.Session,
					map[string]string{"session": cfg.Session, "name": name})
			}},
		&cobra.Command{Use: "list", Short: "List saved autofill profiles",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/autofill/list", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
	)
	return cmd
}

// ---- Tab Strip Dashboard ----

func newTabStripCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "tabstrip", Short: "Visual tab strip dashboard"}
	cmd.AddCommand(
		&cobra.Command{Use: "show", Short: "Open tab strip",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/tabstrip", cfg.Session, nil)
			}},
		&cobra.Command{Use: "close", Short: "Close tab strip",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/tabstrip/close", cfg.Session, nil)
			}},
	)
	return cmd
}

// ---- Screen Recording ----

func newScreenRecCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "screenrec", Short: "Screen recording via periodic screenshots"}

	var fps int
	startCmd := &cobra.Command{Use: "start", Short: "Start recording (default 2 FPS)",
		RunE: func(_ *cobra.Command, _ []string) error {
			resp, err := post(cfg.DaemonAddr, "/screenrec/start",
				map[string]any{"session": cfg.Session, "fps": fps})
			if err != nil { return err }
			return printResp(resp)
		}}
	startCmd.Flags().IntVar(&fps, "fps", 2, "Frames per second (1-10)")

	cmd.AddCommand(
		startCmd,
		&cobra.Command{Use: "stop", Short: "Stop recording",
			RunE: func(_ *cobra.Command, _ []string) error {
				resp, err := post(cfg.DaemonAddr, "/screenrec/stop", map[string]string{"session": cfg.Session})
				if err != nil { return err }
				return printResp(resp)
			}},
		&cobra.Command{Use: "play", Short: "Play back recorded frames in-browser",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/screenrec/play", cfg.Session, nil)
			}},
	)
	return cmd
}

// ---- Page Translation ----

func newTranslateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "translate", Short: "Page translation overlay with original text on hover"}
	cmd.AddCommand(
		&cobra.Command{Use: "apply [lang]", Short: "Apply translation overlay (default: es)",
			Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				lang := "es"
				if len(args) > 0 { lang = args[0] }
				return sendSession(cfg.DaemonAddr, "/translate", cfg.Session,
					map[string]string{"session": cfg.Session, "lang": lang})
			}},
		&cobra.Command{Use: "clear", Short: "Remove translation overlay",
			RunE: func(_ *cobra.Command, _ []string) error {
				return sendSession(cfg.DaemonAddr, "/translate/clear", cfg.Session, nil)
			}},
	)
	return cmd
}

// suppress unused import warning.
var _ = fmt.Sprintf
