# JakLingko Client Go, CLI & MCP Server

A zero-dependency Go client library, command-line interface, and Model Context Protocol (MCP) server for the **JakLingko** public transit ecosystem in Jakarta (TransJakarta BRT, MRT Jakarta, LRT Jakarta, and KRL Commuter Line).

> ⚠️ **Disclaimer**: This project is an unofficial, community-driven reverse-engineering implementation developed strictly for **educational, security research, and interoperability purposes**. It is **not** affiliated with, endorsed by, or sponsored by PT JakLingko Indonesia, TransJakarta, MRT Jakarta, LRT Jakarta, or PT Kereta Commuter Indonesia. Use responsibly in compliance with all applicable terms of service.

---

## Highlights

* **Pure Standard Library**: Zero third-party dependencies (`net/http`, `crypto/aes`, `crypto/cipher`, `crypto/sha256`).
* **Multi-Account Interface Architecture**: Each `jaklingko.Client` manages its own isolated account state, in-memory tokens, and file session.
* **AES-256-CBC PBKDF2 Transport**: Full replication of the official Android encryption layer with PBKDF2 key derivation.
* **Overridable Base URL & Salts**: Configurable via functional options or environment variables (`JAKLINGKO_BASE_URL`, `JAKLINGKO_CIPHER_SALT`, `JAKLINGKO_DETAIL_SALT`) with automatic built-in fallbacks.
* **Complete Transit Flow**: Station discovery → Multimodal fare routing → Instant QRIS booking → Live turnstile gate barcode generation (`JLI#...`) → Peer-to-peer ticket transfer.
* **Stdio MCP Server**: Plug-and-play binary for Claude Desktop, Cursor, Antigravity, OpenCode, or any MCP-compatible AI agent.
* **Ultra-Lightweight Binaries**: Automated UPX compression reduces multi-platform release binaries to minimal footprints.

---

## Quick Installation

### Linux / macOS (Automated One-Liner)
```bash
curl -fsSL https://raw.githubusercontent.com/bardiz12/jaklingko-client/main/install.sh | bash
```

### Windows (PowerShell)
```powershell
powershell -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/bardiz12/jaklingko-client/main/install.ps1 | iex"
```

The script automatically downloads the latest release matching your OS and architecture, installs `jaklingko-cli` and `jaklingko-mcp` into your system `$PATH`, and verifies execution.

---

## Using as a Go Module

Install the package directly into your Go project:

```bash
go get github.com/bardiz12/jaklingko-client@latest
```

### Basic Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/bardiz12/jaklingko-client/pkg/jaklingko"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    client := jaklingko.New(
        jaklingko.WithSessionFile(".session.json"),
    )

    // Authenticate if no active session exists
    if !client.HasValidSession() {
        _, err := client.Login(ctx, "081234567890", "your_password")
        if err != nil {
            log.Fatalf("Login failed: %v", err)
        }
    }

    // Search transit stops
    stops, err := client.SearchLocation(ctx, "Dukuh Atas", 5)
    if err != nil {
        log.Fatalf("Search failed: %v", err)
    }

    for _, s := range stops {
        fmt.Printf("Stop: %s [%.5f, %.5f]\n", s.GetDisplayName(), s.GetLat(), s.GetLng())
    }
}
```

---

## Directory Layout

```
jaklingko-client/
├── bin/
│   ├── jaklingko-cli       # Dedicated CLI binary
│   └── jaklingko-mcp       # Dedicated MCP Server binary
├── cmd/
│   ├── cli/
│   │   └── main.go         # CLI runner entrypoint
│   └── mcp/
│       └── main.go         # MCP Server entrypoint (JSON-RPC 2.0 stdio)
├── pkg/
│   ├── jaklingko/          # Core SDK interface and client implementation
│   │   ├── client.go       # Interface and functional options
│   │   ├── client_impl.go  # Isolated client struct & HTTP transport
│   │   ├── auth.go         # Authentication and session lifecycle
│   │   ├── location.go     # Transit stops and location discovery
│   │   ├── fare.go         # Multimodal routing and fare calculation
│   │   ├── booking.go      # Ticket booking, active tickets & turnstile QRs
│   │   ├── qr.go           # Gate QR refresh, validation, tap direction & watch loop
│   │   └── session.go      # Session persistence (0600 file permissions)
│   ├── crypto/             # AES-256-CBC & PBKDF2 cipher engine
│   └── models/             # Request/response DTO structures
├── install.sh              # Linux & macOS installer
├── install.ps1             # Windows installer
├── SKILL.md                # Plug-and-play AI assistant skill specification
├── .goreleaser.yaml        # Multi-platform release configuration with UPX
└── go.mod
```

---

## Local Compilation

```bash
# Build standalone binaries
go build -o bin/jaklingko-cli cmd/cli/main.go
go build -o bin/jaklingko-mcp cmd/mcp/main.go

# Run tests
go test -v ./...
```

---

## CLI Usage (`jaklingko-cli`)

```bash
# Search route from Dukuh Atas to Monas
jaklingko-cli -origin "Dukuh Atas" -destination "Monas"

# Book route option 1 directly using QRIS
jaklingko-cli -origin "Dukuh Atas" -destination "Monas" -book-option 1

# List active paid tickets
jaklingko-cli -active

# View complete itinerary details & legs for a specific ticket
jaklingko-cli -ticket-detail "T-DBNEP6QDGIPD7T9HIF1G"

# Generate turnstile gate tap-in barcode (all transit legs)
jaklingko-cli -gate-qr "T-DBNEP6QDGIPD7T9HIF1G"

# Generate turnstile gate QR for a specific leg ID
jaklingko-cli -gate-qr "T-DBNEP6QDGIPD7T9HIF1G" -leg-id 20729890

# Keep regenerating the gate QR every 20s, like the mobile app (Ctrl+C to stop)
jaklingko-cli -gate-qr "T-DBNEP6QDGIPD7T9HIF1G" -watch

# Watch for a bounded number of cycles with a custom interval
jaklingko-cli -gate-qr "T-DBNEP6QDGIPD7T9HIF1G" -watch -qr-max-cycles 5 -qr-interval 20s

# Validate a scanned gate QR (tap-in/tap-out direction is decided server-side)
jaklingko-cli -gate-qr "T-DBNEP6QDGIPD7T9HIF1G" -validate-qr "JLI#..."

# Bulk active tickets inspection with all gate QRs generated
jaklingko-cli -active -all-qr

# Check pending invoices awaiting payment
jaklingko-cli -pending

# Look up a transfer recipient by phone (read-only)
jaklingko-cli -lookup-phone "081234567890"

# Transfer an active ticket to another JakLingko user (asks for confirmation + PIN)
jaklingko-cli -transfer-ticket "T-DBNEP6QDGIPD7T9HIF1G" -to "081234567890"

# Non-interactive transfer (e.g. automation): PIN via flag or JAKLINGKO_PIN
jaklingko-cli -transfer-ticket "T-DBNEP6QDGIPD7T9HIF1G" -to "081234567890" -yes -format json

# Output standard JSON (ideal for piping and scripting)
jaklingko-cli -active -format json
jaklingko-cli -ticket-detail "T-DBNEP6QDGIPD7T9HIF1G" -format json
jaklingko-cli -gate-qr "T-DBNEP6QDGIPD7T9HIF1G" -format json

# Clear saved local session
jaklingko-cli -logout
```

### Ticket Transfer

Ticket transfer moves an active ticket from your account to another registered JakLingko user. It is **irreversible**: once transferred, the ticket disappears from the sender's account.

The flow mirrors the Android app:

1. `GET auth/v3/profile/by-phone/{phone}` resolves the recipient (`user_id`, `name`, `phone`, `lang`).
2. `POST ticket/v3/ticket/transfer` (encrypted body) sends `{pin, destination_uid, ticket_id, phone, firstname, lastname, language}`, where `ticket_id` is the ticket long ID (`T-...`).
3. Success is reported by `message == "SUCCESS"`.

Client-side guards reject transfers to your own number and tickets already in use (`in_journey` / `is_used`) before the request is sent. The PIN is the sender's 6-digit account PIN, supplied via `-pin` or `JAKLINGKO_PIN`. In `-format json` mode, `-yes` is required so an automated run cannot silently confirm an irreversible transfer.

### Gate QR Lifecycle

The mobile app (v1.6.22) treats a turnstile QR as short-lived: `TicketQrFragment`,
`ScanTicketActivity` and `ABTQRCodeActivity` each run a `CountDownTimer(20000L, 1000L)`
and request a new QR when it expires. The SDK mirrors this with
`DefaultQRRefreshInterval = 20 * time.Second`.

| Step | Endpoint | Body | Notes |
|------|----------|------|-------|
| Generate | `POST ticket/v3/qr-activation/generate` | `{"ticket_id":N,"legs_id":N}` | Used for the first QR and for every regeneration |
| Refresh | `POST ticket/v3/qr-activation/refresh` | `{"ticket_id":N}` | Declared in the app's Retrofit interface but unused in v1.6.22 |
| Validate | `POST ticket/v3/qr-activation/validate` | `{"ticket_id":N,"legs_id":N,"qr_string":"..."}` | Single endpoint for both tap-in and tap-out |

Validation returns a direction string: `CHECKIN` means tap-in, any other value
means tap-out. The app never compares against a tap-out constant, and the server
derives the direction from the ticket's journey state (`in_journey`), so the
client should treat the direction as advisory. Failures surface codes such as
`INVALID_QR`, `TICKET_NOT_FOUND`, `LEGS_NOT_FOUND`, `ALREADY_USED`,
`SCAN_IN_FIRST` and `SCAN_FAILED`.

`ResolveTapTarget` picks the ticket/leg pair automatically: the next unpassed
transit leg for a tap-in, or the leg currently in journey for a tap-out.

---

## Model Context Protocol (MCP) Server

The binary `jaklingko-mcp` is a standard JSON-RPC 2.0 stdio MCP server designed for integration with AI assistants (Claude Desktop, Cursor, Antigravity, OpenCode).

### Configuration (`claude_desktop_config.json`)

```json
{
  "mcpServers": {
    "jaklingko": {
      "command": "jaklingko-mcp",
      "args": ["-session", "~/.jaklingko-session.json"],
      "env": {
        "JAKLINGKO_USER": "6281234567890",
        "JAKLINGKO_PASS": "your_password"
      }
    }
  }
}
```

### Available MCP Tools

| Tool Name | Parameters | Description |
|-----------|------------|-------------|
| `search_transit_locations` | `query` (string), `limit` (int) | Search for transit stops, stations, or landmarks |
| `get_transit_routes` | `origin` (string), `destination` (string), `sort` (enum) | Search integrated multimodal routes, fares, and booking payloads |
| `book_transit_ticket` | `raw_payload` (object) or field parameters | Book ticket directly with instant QRIS payment |
| `get_pending_tickets` | _(none)_ | Retrieve pending unpaid invoices & QRIS codes |
| `get_active_tickets` | `include_qr` (bool, optional) | Retrieve list of active paid transit tickets |
| `get_ticket_detail` | `ticket_long_id` (string, required) | Retrieve full itinerary, legs, stations, and leg IDs for a ticket |
| `generate_gate_qr` | `ticket_long_id` (string, required), `leg_id` (int, optional) | Generate turnstile tap-in barcode (`JLI#...`) for transit gates |
| `refresh_gate_qr` | `ticket_long_id` (string, required), `leg_id` (int, optional) | Request a fresh gate QR via `qr-activation/refresh` |
| `validate_gate_qr` | `ticket_long_id`, `qr_string` (required), `leg_id` (optional) | Validate a scanned gate QR; returns raw result + `TAP_IN`/`TAP_OUT` |
| `watch_gate_qr` | `ticket_long_id` (required), `leg_id`, `interval_seconds` (default 20), `max_cycles` (default 1) | Regenerate the gate QR on the app's 20s cycle and return every frame |
| `get_payment_channels` | _(none)_ | List available payment channels |
| `pay_fello_ticket` | `fello_url` (string, required), `pin` (string, optional) | Complete an automated headless Fello e-wallet payment |
| `get_balance` | `channel_id` (string, optional) | Check linked e-wallet balances |
| `get_profile_by_phone` | `phone` (string, required) | Resolve a registered user by phone before transferring a ticket |
| `transfer_ticket` | `ticket_long_id` (string), `phone` (string), `pin` (string, optional), `confirm` (bool, required) | Transfer an active ticket to another user (irreversible) |

---

## Configuration & Environment Variables

| Variable | CLI / MCP Flag | Description | Default |
|----------|----------------|-------------|---------|
| `JAKLINGKO_USER` | `-user` | Registered phone number / username | _(none)_ |
| `JAKLINGKO_PASS` | `-pass` | Account password | _(none)_ |
| `JAKLINGKO_BASE_URL` | `-base-url` | JakLingko Gateway API Base URL | `https://api-sapps.jaklingkoindonesia.co.id` |
| `JAKLINGKO_CIPHER_SALT` | `-cipher-salt` | Payload encryption salt | Built-in fallback |
| `JAKLINGKO_DETAIL_SALT` | `-detail-salt` | Ticket detail encryption salt | Built-in fallback |
| `JAKLINGKO_PIN` | `-pin` | 6-digit account PIN for ticket transfer | _(none)_ |

---

## License

This project is released under the [MIT License](LICENSE).
