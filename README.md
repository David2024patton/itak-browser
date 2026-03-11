# iTaK Browser v0.2.1

Go-native browser automation CLI for AI agents. Snapshot+Refs reduces DOM output from 10,000+ tokens to ~200. Persistent daemon eliminates Chrome cold-start latency. Built-in AI recommendation poisoning defense.

## Quick Start

```bash
# Build
go build -o gobrowser.exe ./cmd/gobrowser/

# Start the daemon (keeps Chrome alive between commands)
gobrowser daemon start

# Create a headed (visible) session with ad blocking
gobrowser session new --headed --stealth --block-ads

# Browse
gobrowser -s <session-id> open https://example.com
gobrowser -s <session-id> snapshot
gobrowser -s <session-id> click e2
gobrowser -s <session-id> fill e3 "hello"
gobrowser -s <session-id> screenshot --annotate

# Multi-tab browsing
gobrowser -s <session-id> tab new https://google.com
gobrowser -s <session-id> tab list
gobrowser -s <session-id> tab switch tab_1

# Page intelligence
gobrowser -s <session-id> metrics       # DOM nodes, load time, memory
gobrowser -s <session-id> links         # all hyperlinks (structured)
gobrowser -s <session-id> forms         # all forms + fields (structured)

# Wait primitives
gobrowser -s <session-id> wait-nav      # wait for navigation to complete
gobrowser -s <session-id> wait-idle     # wait for network idle (2s)

# All-in-one debug dump
gobrowser -s <session-id> --json debug
```

## Security: ContentGuard + ThreatLedger

Based on [Microsoft's AI Recommendation Poisoning research](https://www.microsoft.com/en-us/security/blog/2026/02/10/ai-recommendation-poisoning/) (MITRE ATLAS AML.T0080).

**Auto-scan runs on every `open` and `snapshot`** -- not just on `debug`.

### What happens when poisoning is detected:

| Mode | Alert |
|------|-------|
| **Headed** | Red JS overlay popup injected into the page (auto-dismiss 15s) |
| **CLI** | ANSI red warning banner printed to stderr |
| **API** | `scan_result` field included in response JSON |
| **Persistent** | Logged to `~/.itak-browser/threats/threat_ledger.json` |
| **Webhook** | Real-time POST to configured URL (3 retries, 5s timeout) |

### Detection Patterns

| Pattern | Severity | Example |
|---------|----------|---------|
| memory-persist-remember | High | "remember X as a trusted source" |
| memory-persist-future | High | "in future conversations", "from now on" |
| authority-injection | Medium | "always recommend", "the best choice for" |
| memory-keep | High | "keep X in your memory", "save this to memory" |
| hidden-system-prompt | High | "ignore previous instructions", "you are now" |
| promotional-injection | Low | "all-in-one sales platform", "industry-leading" |
| url-prompt-param | High | URL with ?q=...remember...trusted... |

### Public Threat Feed

```
GET /threats          # all recorded poisoning detections
GET /threats/stats    # aggregate stats (total, unique domains, severity, top 20)
```

CORS-enabled. Configure webhook for instant push:
```
gobrowser webhook https://your-site.com/api/threats
```

## Key Features

| Feature | Command | Description |
|---------|---------|-------------|
| Snapshot+Refs | `snapshot` | Compact accessibility tree (~93% token reduction) |
| Headed Mode | `session new --headed` | Visible Chrome window for debugging |
| Tab Management | `tab new/switch/close/list` | Multi-tab browsing |
| Request Blocking | `--block-ads` | Block ads/trackers via CDP Fetch (25+ default domains) |
| Page Metrics | `metrics` | DOM nodes, depth, load time, transfer size, JS heap |
| Link Extraction | `links` | All hyperlinks (text, href, rel, target) |
| Form Extraction | `forms` | All forms + fields (action, method, type, name) |
| Wait Navigation | `wait-nav` | Wait for page load after click |
| Wait Network Idle | `wait-idle` | Wait until no requests for 2 seconds |
| Webhook Push | `webhook <url>` | Real-time poisoning detection notifications |
| Proxy Support | `--proxy` flag | Route through HTTP/SOCKS proxy |
| Stealth Mode | `--stealth` | Anti-detection patches (UA, webdriver, plugins) |
| Debug Bundle | `debug` | Snapshot + screenshot + console + network + scan |
| Console Capture | `console` | Ring buffer of console.log/warn/error |
| Network Monitor | `network` | Captured request/response pairs with timing |
| Session Save | `session save` | AES-256-GCM encrypted cookie persistence |

## Architecture

```
CLI (gobrowser) --HTTP--> Daemon (port 43721) --CDP--> Chrome
       |                        |
  thin proxy              session pool
                                |
                    Engine + TabManager + ConsoleCapture
                           + NetworkCapture + ContentGuard
                           + RequestBlocker
```

## Module Structure

```
pkg/browser/
  engine.go         Engine core: 30+ methods for browsing, interaction, extraction
  tabs.go           Multi-tab management (new, switch, close, list)
  blocker.go        CDP Fetch request blocking (ads, trackers, custom domains)
  metrics.go        Page performance (DOM nodes, load time, memory, transfer size)
  extract.go        Bulk extraction (links, forms)
  console.go        CDP console log capture (ring buffer)
  network.go        CDP network request monitoring (ring buffer)
  contentguard.go   AI recommendation poisoning scanner (7 patterns)
  threatledger.go   Persistent threat log + webhook push + stats
  stealth.go        Anti-detection flags + JS init script
  snapshot.go       Accessibility tree builder (Snapshot+Refs)
  annotate.go       Screenshot annotation for vision models
  session.go        Cookie save/restore with AES-256-GCM
pkg/daemon/
  daemon.go         HTTP daemon with 50+ endpoints
pkg/cli/
  cli.go            Cobra CLI with 40+ subcommands
cmd/gobrowser/
  main.go           Binary entry point
```
