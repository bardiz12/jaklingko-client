---
name: jaklingko
description: Query Jakarta integrated public transit (TransJakarta BRT, MRT Jakarta, LRT, KRL Commuter Line), calculate multimodal fares, book tickets with instant QRIS payment, check pending invoices, and inspect active tickets with per-operator turnstile barcodes (JLI#...). Use when the user asks about transit in Jakarta, TransJakarta bus schedules/routes, MRT Jakarta fares, buying JakLingko tickets, checking QRIS payment strings, or getting station tap-in barcodes.
trigger: jaklingko|transjakarta|mrt jakarta|lrt jakarta|krl jakarta|jaklingko route|jaklingko ticket|tiket jaklingko|rute transjakarta|tarif mrt|jadwal jaklingko|pesan tiket transit|qris jaklingko|turnstile jaklingko|gate qr
---

# JakLingko Transit & Ticketing Assistant Skill

A plug-and-play AI agent skill for interacting with the **JakLingko** integrated transit network in the Jakarta metropolitan area (TransJakarta BRT, MRT Jakarta, LRT Jakarta, KRL Commuter Line).

---

## 1. Quick Installation & Self-Update (for AI Agents)

AI agents can verify binary presence, check version info, and automatically install or self-update `jaklingko-mcp` and `jaklingko-cli` at runtime.

### Version Inspection
Check installed version and build metadata:
```bash
# Standard output
jaklingko-cli -version
jaklingko-mcp -version

# Machine-readable JSON output
jaklingko-cli -version -format json
```

### Automated Installation & Self-Update
If binaries are missing or an update is needed, execute the one-liner installer. The installer detects OS and CPU architecture, downloads the latest release from GitHub, unpacks UPX-optimized binaries, atomically replaces old executables in `~/.local/bin` (or `/usr/local/bin` / `%LOCALAPPDATA%`), and preserves existing session files and tokens.

#### Linux / macOS
```bash
curl -fsSL https://raw.githubusercontent.com/bardiz12/jaklingko-client/main/install.sh | bash
```

#### Windows (PowerShell)
```powershell
powershell -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/bardiz12/jaklingko-client/main/install.ps1 | iex"
```

---

## 2. Configuration & Runtime Overrides

Both `jaklingko-mcp` and `jaklingko-cli` support runtime customization via flags or environment variables:

| Setting | Flag | Environment Variable | Default / Fallback |
| :--- | :--- | :--- | :--- |
| **API Gateway** | `-base-url` | `JAKLINGKO_BASE_URL` | `https://api-sapps.jaklingkoindonesia.co.id` |
| **Cipher Salt** | `-cipher-salt` | `JAKLINGKO_CIPHER_SALT` | Built-in PBKDF2 payload salt |
| **Detail Salt** | `-detail-salt` | `JAKLINGKO_DETAIL_SALT` | Built-in PBKDF2 detail salt |
| **User-Agent** | `-user-agent` | `JAKLINGKO_USER_AGENT` | `okhttp/4.12.0` |
| **Fello PIN** | `-fello-pin` | `JAKLINGKO_FELLO_PIN` / `FELLO_PIN` | — |
| **Session Path** | `-session` | — | `~/.jaklingko-session.json` |
| **Username / Phone** | `-user` | `JAKLINGKO_USER` / `JAKLINGKO_PHONE` | — |
| **Password** | `-pass` | `JAKLINGKO_PASS` / `JAKLINGKO_PASSWORD` | — |

### MCP Server Configuration Example
Add the server definition to your MCP client configuration (e.g. `claude_desktop_config.json`, Cursor MCP settings, or Antigravity):

```json
{
  "mcpServers": {
    "jaklingko": {
      "command": "jaklingko-mcp",
      "args": [
        "-session", "~/.jaklingko-session.json"
      ],
      "env": {
        "JAKLINGKO_USER": "6281234567890",
        "JAKLINGKO_PASS": "your_password",
        "JAKLINGKO_FELLO_PIN": "123456"
      }
    }
  }
}
```

*Note: If `jaklingko-mcp` is not in global `$PATH`, specify the full binary path (e.g., `~/.local/bin/jaklingko-mcp` or `/usr/local/bin/jaklingko-mcp`).*

---

## 3. Communication Protocol Guide

* **Transport**: Standard JSON-RPC 2.0 over `stdin` / `stdout`.
* **Output Purity**: All internal diagnostics, logging, and connection status are routed strictly to `stderr`. The `stdout` stream emits 100% compliant JSON-RPC 2.0 messages.
* **Session Lifecycle**: The server persists active JWT sessions to the configured session file (`0600` file permissions). On subsequent runs, it reuses the authenticated session without repeatedly hammering the login endpoint.
* **Auto-Login**: If no valid session exists on startup, the server automatically authenticates using `JAKLINGKO_USER` and `JAKLINGKO_PASS` environment variables.

---

## 4. MCP Tools Reference

The server exposes 10 modular transit tools:

### `search_transit_locations`
* **Description**: Search for transit stops, BRT halts, MRT/LRT/KRL train stations, or landmarks in the Jakarta metropolitan area.
* **Arguments**:
  * `query` (string, required): Station or landmark name (e.g. `"Dukuh Atas"`, `"Monas"`, `"Bundaran HI"`, `"Lebak Bulus"`).
  * `limit` (integer, optional, default: 5): Max results to return.
* **Output**: Array of location objects with `label`, `latitude`, `longitude`, `pto_name`, and `modes`.

### `get_transit_routes`
* **Description**: Calculate multimodal integrated routes, durations, transfers, and fares between origin and destination.
* **Arguments**:
  * `origin` (string, required): Origin station or landmark.
  * `destination` (string, required): Destination station or landmark.
  * `sort` (string, optional, default: `"cheapest"`): Priority filter: `"cheapest"`, `"fastest"`, or `"least_transfer"`.
* **Output**: Ranked route options including segment-by-segment legs (Walk, TransJakarta, MRT, LRT), total fare (in IDR), duration, and a pre-constructed `booking_payload`.

### `book_transit_ticket`
* **Description**: Book an integrated transit ticket for a selected route with instant QRIS payment.
* **Arguments**:
  * `raw_payload` (object, optional): Direct `booking_payload` object from `get_transit_routes`.
  * Alternatively pass fields individually: `itineraries_id`, `amount`, `origin_label`, `destination_label`, `service_ids`.
* **Output**: Booking confirmation containing `invoice_id`, `transaction_number`, `amount`, payment deadline, and the EMVCo QRIS string (`qr_content`).

### `get_pending_tickets`
* **Description**: List unpaid invoices awaiting QRIS payment settlement.
* **Arguments**: None.
* **Output**: Array of pending invoices with amount, date, route labels, and status.

### `get_active_tickets`
* **Description**: List confirmed, active paid tickets for the user.
* **Arguments**:
  * `include_qr` (boolean, optional, default: `false`): If true, generates turnstile gate QRs for all transit legs immediately.
* **Output**: Array of active tickets with `ticket_longid`, route labels, status, and expiration date.

### `get_ticket_detail`
* **Description**: Retrieve detailed itinerary segments, stations, transfers, fares, and leg IDs for an active ticket.
* **Arguments**:
  * `ticket_long_id` (string, required): The ticket long ID (e.g., `"T-DBNEP6QDGIPD7T9HIF1G"`).
* **Output**: Full itinerary legs with `leg_id`, `mode`, `agency`, `line`, origin/destination stops, and active status.

### `generate_gate_qr`
* **Description**: Generate the official turnstile barcode (`JLI#...`) for tapping in at transit entry gates (TransJakarta BRT, MRT Jakarta, LRT, KRL).
* **Arguments**:
  * `ticket_long_id` (string, required): Active ticket long ID.
  * `leg_id` (integer, optional): Specific transit leg ID. If omitted or 0, generates QRs for all transit legs.
* **Output**: Array of gate QR objects containing `leg_id`, `operator`, `origin`, `destination`, `qr_string` (`JLI#...`), and expiration date.

### `get_payment_channels`
* **Description**: List payment channels supported by JakLingko.
* **Arguments**: None.

### `get_balance`
* **Description**: Check e-wallet balance (Fello, OVO, etc.) for linked JakLingko payment channels. If `channel_id` is specified (e.g. `"5"` for Fello), returns balance for that channel; otherwise returns balances for all linked e-wallets.
* **Arguments**:
  * `channel_id` (string, optional): Specific payment channel ID (e.g. `"5"`). If omitted, returns all linked e-wallets.
* **Output**: Balance details object or array of linked e-wallet balance objects.

### `pay_fello_ticket`
* **Description**: Complete an automated headless e-wallet payment for a JakLingko transit ticket using Fello and a 6-digit PIN. Automatically verifies PIN credentials with Fello and finalizes the settlement with JakLingko core API.
* **Arguments**:
  * `fello_url` (string, required): Fello payment gateway URL (obtained from `book_transit_ticket` result `qr_web_url` when booked with payment channel `5`).
  * `pin` (string, required): 6-digit Fello e-wallet PIN.
* **Output**: Status confirmation object.

---

## 5. Agent Decision Workflows

### Workflow A: Route Search & Booking
1. When a user asks how to get from point A to point B:
   - Call `get_transit_routes(origin, destination)`.
   - Present the top route options (duration, transfers, total fare in IDR).
2. When the user confirms booking:
   - Call `book_transit_ticket` with the pre-generated `booking_payload` of the selected route.
   - Display the `invoice_id`, total amount, and the `qr_content` (EMVCo QRIS string) for payment.

### Workflow B: Gate Tap-In QR Barcode Retrieval
When the user has paid and is at the station or asks for their gate barcode:
1. Call `get_active_tickets()`.
2. Find the active ticket matching their journey.
3. Call `generate_gate_qr(ticket_long_id, leg_id)` to get the `JLI#...` turnstile barcode for the appropriate transit leg.
4. Provide the QR string and remind them of the expiration time (~03:00 WIB the next day).

### Workflow C: Payment Verification
When the user asks if their QRIS payment succeeded:
1. Call `get_active_tickets()`. If the ticket appears with status `"active"`, payment was verified!
2. If not yet active, call `get_pending_tickets()` to check if the invoice is still awaiting settlement.

### Workflow D: Automated Headless Fello Payment
When the user wants to buy a transit ticket using their linked Fello e-wallet:
1. Book the ticket specifying payment channel 5 (Fello): `book_transit_ticket` with `payment_channel="5"`.
2. Inspect `qr_web_url` in the booking response.
3. Prompt or ask user for their 6-digit Fello PIN (or use provided PIN).
4. Call `pay_fello_ticket(fello_url, pin)`.
5. Call `get_active_tickets(include_qr=true)` to immediately obtain the active ticket and turnstile tap-in barcode (`JLI#...`)!

---

## 6. CLI Fallback (Direct Shell Execution)

If the MCP client cannot communicate via stdio, execute commands directly via `jaklingko-cli`:

```bash
# Search routes
jaklingko-cli -origin "Dukuh Atas" -destination "Monas" -format json

# Check all linked e-wallet balances
jaklingko-cli -balance -format json

# Check balance for specific channel (e.g. Channel 5 for Fello)
jaklingko-cli -balance -channel 5

# Book directly with Fello channel and pay headlessly in one step
jaklingko-cli -origin "Dukuh Atas" -destination "Bundaran HI" -book-option 1 -channel 5 -fello-pin 123456

# Pay an existing Fello gateway purchase URL headlessly
jaklingko-cli -pay-fello "https://pay-fello.fellopay.id/Api/Wallet/V1/Payment/Purchase/Request?tID=..." -fello-pin 123456

# List active tickets
jaklingko-cli -active -format json

# View ticket details
jaklingko-cli -ticket-detail "T-DBNEP6QDGIPD7T9HIF1G" -format json

# Generate gate turnstile tap-in QR
jaklingko-cli -gate-qr "T-DBNEP6QDGIPD7T9HIF1G" -format json
```
