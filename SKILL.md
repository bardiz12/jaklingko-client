---
name: jaklingko
description: Query Jakarta integrated public transit (TransJakarta BRT, MRT Jakarta, LRT, KRL Commuter Line), calculate multimodal fares, book tickets with instant QRIS payment, check pending invoices, and inspect active tickets with per-operator turnstile barcodes (JLI#...). Use when the user asks about transit in Jakarta, TransJakarta bus schedules/routes, MRT Jakarta fares, buying JakLingko tickets, checking QRIS payment strings, or getting station tap-in barcodes.
trigger: jaklingko|transjakarta|mrt jakarta|lrt jakarta|krl jakarta|jaklingko route|jaklingko ticket|tiket jaklingko|rute transjakarta|tarif mrt|jadwal jaklingko|pesan tiket transit|qris jaklingko|turnstile jaklingko|gate qr
---

# JakLingko Transit & Ticketing Assistant Skill

A plug-and-play AI agent skill for interacting with the **JakLingko** integrated transit network in the Jakarta metropolitan area (TransJakarta BRT, MRT Jakarta, LRT Jakarta, KRL Commuter Line).

---

## 1. Quick Installation (Self-Setup for AI Agents)

If `jaklingko-mcp` or `jaklingko-cli` is not yet installed in the current environment, execute the automated cross-platform installer:

### Linux / macOS (One-Liner)
```bash
curl -fsSL https://raw.githubusercontent.com/bardiz12/jaklingko-client/main/install.sh | bash
```

### Windows (PowerShell)
```powershell
powershell -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/bardiz12/jaklingko-client/main/install.ps1 | iex"
```

The installer detects OS and architecture, downloads the latest binary release, places `jaklingko-cli` and `jaklingko-mcp` into your system `$PATH`, and verifies execution.

---

## 2. MCP Server Configuration

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
        "JAKLINGKO_PASS": "your_password"
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

The server exposes 8 modular transit tools:

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

---

## 6. CLI Fallback (Direct Shell Execution)

If the MCP client cannot communicate via stdio, execute commands directly via `jaklingko-cli`:

```bash
# Search routes
jaklingko-cli -origin "Dukuh Atas" -destination "Monas" -format json

# List active tickets
jaklingko-cli -active -format json

# View ticket details
jaklingko-cli -ticket-detail "T-DBNEP6QDGIPD7T9HIF1G" -format json

# Generate gate turnstile tap-in QR
jaklingko-cli -gate-qr "T-DBNEP6QDGIPD7T9HIF1G" -format json
```
