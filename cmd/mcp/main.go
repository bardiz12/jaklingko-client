package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bardiz12/jaklingko-client/pkg/jaklingko"
	"github.com/bardiz12/jaklingko-client/pkg/models"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// JSON-RPC 2.0 Structures
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP Content structures
type MCPTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MCPToolResult struct {
	Content []MCPTextContent `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

// Tool Definition for tools/list
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

var toolsList = []MCPTool{
	{
		Name:        "search_transit_locations",
		Description: "Search for transit stops, BRT stations, MRT/LRT/KRL train stations, or landmarks in the Jakarta metropolitan area.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Station, bus stop, landmark, or street name (e.g. 'Dukuh Atas', 'Monas', 'Lebak Bulus', 'Bundaran HI')",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of search results to return (default: 5)",
					"default":     5,
				},
			},
			"required": []string{"query"},
		},
	},
	{
		Name:        "get_transit_routes",
		Description: "Search multimodal integrated transit routes and fare calculations between an origin and destination in Jakarta. Returns route options, trip durations, operators (TransJakarta, MRT Jakarta, LRT, KRL), transfer legs, and pre-generated ready-to-use booking payloads.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"origin": map[string]interface{}{
					"type":        "string",
					"description": "Origin station, halte, or landmark name (e.g. 'Dukuh Atas')",
				},
				"destination": map[string]interface{}{
					"type":        "string",
					"description": "Destination station, halte, or landmark name (e.g. 'Monas')",
				},
				"sort": map[string]interface{}{
					"type":        "string",
					"description": "Sort priority: 'cheapest', 'fastest', or 'least_transfer' (default: 'cheapest')",
					"enum":        []string{"cheapest", "fastest", "least_transfer"},
					"default":     "cheapest",
				},
			},
			"required": []string{"origin", "destination"},
		},
	},
	{
		Name:        "book_transit_ticket",
		Description: "Book an integrated transit ticket for a selected route with QRIS payment. Generates an active ticket order, invoice ID, and an EMVCo QRIS string for instant payment.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"itineraries_id": map[string]interface{}{
					"type":        "string",
					"description": "Itinerary ID obtained from get_transit_routes option",
				},
				"amount": map[string]interface{}{
					"type":        "string",
					"description": "Total fare amount string (e.g. '3000')",
				},
				"origin_label": map[string]interface{}{
					"type":        "string",
					"description": "Origin station label",
				},
				"destination_label": map[string]interface{}{
					"type":        "string",
					"description": "Destination station label",
				},
				"service_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Service IDs array from route fare",
				},
				"payment_channel": map[string]interface{}{
					"type":        "string",
					"description": "Payment channel ID (default: '4' for QRIS)",
					"default":     "4",
				},
				"raw_payload": map[string]interface{}{
					"type":        "object",
					"description": "Direct BookTicketRequest JSON payload as provided in get_transit_routes.booking_payload",
				},
			},
		},
	},
	{
		Name:        "get_pending_tickets",
		Description: "Retrieve pending unpaid transit invoices. Includes invoice ID, total bill amount, payment status, and EMVCo QRIS payment strings.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "get_active_tickets",
		Description: "Retrieve list of active paid transit tickets for the authenticated user. Returns ticket long IDs, route labels, status, expiration dates, and payment methods. Set include_qr=true to also generate turnstile gate QRs immediately.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"include_qr": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, also retrieves full ticket details and generates turnstile tap-in QRs for all transit legs immediately (default: false for fast response)",
				},
			},
		},
	},
	{
		Name:        "get_ticket_detail",
		Description: "Retrieve detailed itinerary, transit legs, stations/stops, operators (TransJakarta, MRT, LRT, KRL), fares, and leg IDs for a specific active ticket.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_long_id": map[string]interface{}{
					"type":        "string",
					"description": "Active ticket long ID (e.g. 'T-DBNEP6QDGIPD7T9HIF1G')",
				},
			},
			"required": []string{"ticket_long_id"},
		},
	},
	{
		Name:        "generate_gate_qr",
		Description: "Generate turnstile gate QR barcode string (JLI#...) for tapping in at transit stations/haltes (TransJakarta BRT, MRT Jakarta, LRT, KRL). Requires ticket_long_id; optional leg_id to generate for a specific segment.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_long_id": map[string]interface{}{
					"type":        "string",
					"description": "Active ticket long ID (e.g. 'T-DBNEP6QDGIPD7T9HIF1G')",
				},
				"leg_id": map[string]interface{}{
					"type":        "integer",
					"description": "Specific transit leg ID. If omitted or 0, generates QRs for all transit legs.",
				},
			},
			"required": []string{"ticket_long_id"},
		},
	},
	{
		Name:        "refresh_gate_qr",
		Description: "Request a fresh turnstile gate QR for an active ticket via ticket/v3/qr-activation/refresh. The mobile app regenerates QRs every 20 seconds; use watch_gate_qr to mirror that loop. Resolves ticket_long_id to the integer ticket ID automatically.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_long_id": map[string]interface{}{
					"type":        "string",
					"description": "Active ticket long ID (e.g. 'T-DBNEP6QDGIPD7T9HIF1G')",
				},
				"leg_id": map[string]interface{}{
					"type":        "integer",
					"description": "Optional transit leg ID used to resolve the tap direction/leg context.",
				},
			},
			"required": []string{"ticket_long_id"},
		},
	},
	{
		Name:        "validate_gate_qr",
		Description: "Validate a scanned turnstile gate QR against the JakLingko backend (ticket/v3/qr-activation/validate). Returns the raw backend result plus the derived tap direction: CHECKIN = tap-in, anything else = tap-out. The server decides the direction from the ticket's journey state.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_long_id": map[string]interface{}{
					"type":        "string",
					"description": "Active ticket long ID (e.g. 'T-DBNEP6QDGIPD7T9HIF1G')",
				},
				"qr_string": map[string]interface{}{
					"type":        "string",
					"description": "Scanned gate QR string (e.g. 'JLI#...')",
				},
				"leg_id": map[string]interface{}{
					"type":        "integer",
					"description": "Optional transit leg ID. If omitted, the leg is chosen from the ticket state.",
				},
			},
			"required": []string{"ticket_long_id", "qr_string"},
		},
	},
	{
		Name:        "watch_gate_qr",
		Description: "Regenerate the turnstile gate QR on a fixed interval, mirroring the mobile app's 20 second countdown. Returns one frame per cycle. Keep max_cycles small (default 1) because the call blocks for (max_cycles-1) * interval_seconds.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_long_id": map[string]interface{}{
					"type":        "string",
					"description": "Active ticket long ID (e.g. 'T-DBNEP6QDGIPD7T9HIF1G')",
				},
				"leg_id": map[string]interface{}{
					"type":        "integer",
					"description": "Optional transit leg ID. If omitted, the leg is chosen from the ticket state.",
				},
				"interval_seconds": map[string]interface{}{
					"type":        "number",
					"description": "Seconds between regenerations (default: 20, matching the mobile app).",
				},
				"max_cycles": map[string]interface{}{
					"type":        "integer",
					"description": "Number of QR cycles to emit (default: 1, max: 60).",
				},
			},
			"required": []string{"ticket_long_id"},
		},
	},
	{
		Name:        "get_payment_channels",
		Description: "List available payment methods supported by JakLingko (QRIS, OVO, Fello, etc.).",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	},
	{
		Name:        "pay_fello_ticket",
		Description: "Complete an automated headless e-wallet payment for a JakLingko transit ticket using Fello and a 6-digit PIN. Automatically verifies PIN credentials and finalizes the settlement with JakLingko core API.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"fello_url": map[string]interface{}{
					"type":        "string",
					"description": "Fello payment gateway URL (obtained from book_transit_ticket result QRWebURL when booked with channel 5)",
				},
				"pin": map[string]interface{}{
					"type":        "string",
					"description": "6-digit Fello e-wallet PIN. If omitted, server reads from JAKLINGKO_FELLO_PIN or FELLO_PIN environment variable.",
				},
			},
			"required": []string{"fello_url"},
		},
	},
	{
		Name:        "get_balance",
		Description: "Check e-wallet balance (Fello, OVO, etc.) for linked JakLingko payment channels. If channel_id is specified (e.g. '5' for Fello), returns balance for that channel; otherwise returns balances for all linked e-wallets.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"channel_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional payment channel ID (e.g. '5' for Fello). If omitted, returns all linked e-wallets.",
				},
			},
		},
	},
	{
		Name:        "get_profile_by_phone",
		Description: "Look up a registered JakLingko user by phone number (read-only). Returns user_id, name, email, phone, and language. Use this to resolve the transfer recipient before calling transfer_ticket.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"phone": map[string]interface{}{
					"type":        "string",
					"description": "Recipient phone number (e.g. '081234567890'). Local and +62 formats are normalized automatically.",
				},
			},
			"required": []string{"phone"},
		},
	},
	{
		Name:        "transfer_ticket",
		Description: "Transfer an active transit ticket to another registered JakLingko user. IRREVERSIBLE: the ticket leaves the sender's account. Requires confirm=true and the sender's 6-digit account PIN (from the 'pin' argument or JAKLINGKO_PIN env). Tickets already in use and self-transfers are rejected.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"ticket_long_id": map[string]interface{}{
					"type":        "string",
					"description": "Active ticket long ID (e.g. 'T-DBNEP6QDGIPD7T9HIF1G')",
				},
				"phone": map[string]interface{}{
					"type":        "string",
					"description": "Recipient phone number (e.g. '081234567890')",
				},
				"pin": map[string]interface{}{
					"type":        "string",
					"description": "Sender's 6-digit account PIN. If omitted, the server reads JAKLINGKO_PIN from the environment.",
				},
				"confirm": map[string]interface{}{
					"type":        "boolean",
					"description": "Must be true to acknowledge that the transfer is irreversible.",
				},
			},
			"required": []string{"ticket_long_id", "phone", "confirm"},
		},
	},
}

type MCPServer struct {
	client jaklingko.Client
}

func main() {
	// Send all diagnostics and logs to stderr to keep stdout strictly for JSON-RPC 2.0 messages
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	sessionFile := flag.String("session", jaklingko.DefaultSessionFile, "Path to user session file")
	usernameFlag := flag.String("user", "", "JakLingko registered phone/username (or set JAKLINGKO_USER)")
	passwordFlag := flag.String("pass", "", "JakLingko account password (or set JAKLINGKO_PASS)")
	baseURLFlag := flag.String("base-url", "", "Override API base URL (or set JAKLINGKO_BASE_URL)")
	cipherSaltFlag := flag.String("cipher-salt", "", "Override payload encryption salt (or set JAKLINGKO_CIPHER_SALT)")
	detailSaltFlag := flag.String("detail-salt", "", "Override ticket detail encryption salt (or set JAKLINGKO_DETAIL_SALT)")
	userAgentFlag := flag.String("user-agent", "", "Override HTTP User-Agent (or set JAKLINGKO_USER_AGENT)")
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("jaklingko-mcp %s (commit: %s, built: %s)\n", version, commit, date)
		return
	}

	user := *usernameFlag
	if user == "" {
		user = os.Getenv("JAKLINGKO_USER")
		if user == "" {
			user = os.Getenv("JAKLINGKO_PHONE")
		}
	}
	pass := *passwordFlag
	if pass == "" {
		pass = os.Getenv("JAKLINGKO_PASS")
		if pass == "" {
			pass = os.Getenv("JAKLINGKO_PASSWORD")
		}
	}

	opts := []jaklingko.Option{
		jaklingko.WithSessionFile(*sessionFile),
		jaklingko.WithCredentials(user, pass),
	}
	if *baseURLFlag != "" {
		opts = append(opts, jaklingko.WithBaseURL(*baseURLFlag))
	}
	if *cipherSaltFlag != "" {
		opts = append(opts, jaklingko.WithCipherSalt(*cipherSaltFlag))
	}
	if *detailSaltFlag != "" {
		opts = append(opts, jaklingko.WithDetailSalt(*detailSaltFlag))
	}
	if *userAgentFlag != "" {
		opts = append(opts, jaklingko.WithUserAgent(*userAgentFlag))
	}
	client := jaklingko.New(opts...)

	// Attempt auto-login if no active session exists but credentials are provided
	if !client.HasValidSession() && user != "" && pass != "" {
		log.Printf("No active session found. Logging in as user %s...", user)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if _, err := client.Login(ctx, user, pass); err != nil {
			log.Printf("[WARN] Auto-login failed: %v", err)
		} else {
			log.Printf("[OK] Auto-login successful for %s", user)
		}
		cancel()
	}

	server := &MCPServer{client: client}
	server.RunStdio()
}

func (s *MCPServer) RunStdio() {
	reader := bufio.NewReader(os.Stdin)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Read error: %v", err)
			break
		}

		lineStr := strings.TrimSpace(string(line))
		if lineStr == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(lineStr), &req); err != nil {
			s.sendError(nil, -32700, "Parse error: "+err.Error())
			continue
		}

		s.handleRequest(&req)
	}
}

func (s *MCPServer) handleRequest(req *JSONRPCRequest) {
	// Notifications (no ID)
	if req.ID == nil {
		if req.Method == "notifications/initialized" {
			log.Printf("MCP client initialized notification received")
		}
		return
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "ping":
		s.sendResult(req.ID, map[string]interface{}{})
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		s.sendError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func (s *MCPServer) handleInitialize(req *JSONRPCRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "jaklingko-mcp",
			"version": version,
		},
	}
	s.sendResult(req.ID, result)
}

func (s *MCPServer) handleToolsList(req *JSONRPCRequest) {
	result := map[string]interface{}{
		"tools": toolsList,
	}
	s.sendResult(req.ID, result)
}

func (s *MCPServer) handleToolsCall(req *JSONRPCRequest) {
	var callParams struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &callParams); err != nil {
		s.sendToolError(req.ID, "Invalid tools/call params: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch callParams.Name {
	case "search_transit_locations":
		s.toolSearchLocations(ctx, req.ID, callParams.Arguments)
	case "get_transit_routes":
		s.toolGetTransitRoutes(ctx, req.ID, callParams.Arguments)
	case "book_transit_ticket":
		s.toolBookTransitTicket(ctx, req.ID, callParams.Arguments)
	case "get_pending_tickets":
		s.toolGetPendingTickets(ctx, req.ID)
	case "get_active_tickets":
		s.toolGetActiveTickets(ctx, req.ID, callParams.Arguments)
	case "get_ticket_detail":
		s.toolGetTicketDetail(ctx, req.ID, callParams.Arguments)
	case "generate_gate_qr":
		s.toolGenerateGateQR(ctx, req.ID, callParams.Arguments)
	case "refresh_gate_qr":
		s.toolRefreshGateQR(ctx, req.ID, callParams.Arguments)
	case "validate_gate_qr":
		s.toolValidateGateQR(ctx, req.ID, callParams.Arguments)
	case "watch_gate_qr":
		s.toolWatchGateQR(ctx, req.ID, callParams.Arguments)
	case "get_payment_channels":
		s.toolGetPaymentChannels(ctx, req.ID)
	case "pay_fello_ticket":
		s.toolPayFelloTicket(ctx, req.ID, callParams.Arguments)
	case "get_balance":
		s.toolGetBalance(ctx, req.ID, callParams.Arguments)
	case "get_profile_by_phone":
		s.toolGetProfileByPhone(ctx, req.ID, callParams.Arguments)
	case "transfer_ticket":
		s.toolTransferTicket(ctx, req.ID, callParams.Arguments)
	default:
		s.sendToolError(req.ID, fmt.Sprintf("Unknown tool: %s", callParams.Name))
	}
}

func (s *MCPServer) toolSearchLocations(ctx context.Context, id interface{}, args map[string]interface{}) {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		s.sendToolError(id, "Parameter 'query' is required")
		return
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	stops, err := s.client.SearchLocation(ctx, query, limit)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Search location failed: %v", err))
		return
	}

	s.sendToolJSON(id, stops)
}

func (s *MCPServer) toolGetTransitRoutes(ctx context.Context, id interface{}, args map[string]interface{}) {
	originQ, _ := args["origin"].(string)
	destQ, _ := args["destination"].(string)

	if strings.TrimSpace(originQ) == "" || strings.TrimSpace(destQ) == "" {
		s.sendToolError(id, "Parameters 'origin' and 'destination' are required")
		return
	}

	sort := "cheapest"
	if s, ok := args["sort"].(string); ok && s != "" {
		sort = s
	}

	originStops, err := s.client.SearchLocation(ctx, originQ, 3)
	if err != nil || len(originStops) == 0 {
		s.sendToolError(id, fmt.Sprintf("Origin location '%s' not found: %v", originQ, err))
		return
	}
	origin := originStops[0]

	destStops, err := s.client.SearchLocation(ctx, destQ, 3)
	if err != nil || len(destStops) == 0 {
		s.sendToolError(id, fmt.Sprintf("Destination location '%s' not found: %v", destQ, err))
		return
	}
	destination := destStops[0]

	opts := &jaklingko.FareSearchOptions{
		Sort: sort,
	}

	fares, err := s.client.SearchTicketFare(ctx, origin, destination, opts)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to calculate transit routes: %v", err))
		return
	}

	type RouteOptionSummary struct {
		OptionNumber   int                      `json:"option_number"`
		ItineraryID    string                   `json:"itinerary_id"`
		TotalCost      int                      `json:"total_cost"`
		Duration       string                   `json:"duration"`
		DistanceKM     float64                  `json:"distance_km"`
		Origin         string                   `json:"origin"`
		Destination    string                   `json:"destination"`
		Segments       []string                 `json:"segments"`
		BookingPayload models.BookTicketRequest `json:"booking_payload"`
	}

	var summaries []RouteOptionSummary
	for i, f := range fares {
		var segs []string
		for _, leg := range f.Legs {
			op := leg.ShowMode
			if op == "" && leg.Agency != nil {
				op = leg.Agency.Name
			}
			if op == "" {
				op = leg.Mode
			}
			lineInfo := ""
			if leg.Line != nil && leg.Line.Name != "" {
				lineInfo = " (" + leg.Line.Name + ")"
			}
			segs = append(segs, fmt.Sprintf("%s: %s -> %s%s",
				op, leg.Origin.GetDisplayName(), leg.Destination.GetDisplayName(), lineInfo))
		}

		cost := f.PTOCost
		if cost <= 0 {
			cost = f.TotalAmount
		}

		bp := f.BookingPayload
		if bp == nil {
			p := jaklingko.BuildBookingPayload(f, jaklingko.PaymentChannelQRIS)
			bp = &p
		}

		summaries = append(summaries, RouteOptionSummary{
			OptionNumber:   i + 1,
			ItineraryID:    f.ID,
			TotalCost:      cost,
			Duration:       f.GeneralDuration,
			DistanceKM:     f.GeneralDistance,
			Origin:         f.Origin.GetDisplayName(),
			Destination:    f.Destination.GetDisplayName(),
			Segments:       segs,
			BookingPayload: *bp,
		})
	}

	s.sendToolJSON(id, map[string]interface{}{
		"query": map[string]interface{}{
			"origin":      origin.GetDisplayName(),
			"destination": destination.GetDisplayName(),
			"total_found": len(summaries),
		},
		"routes": summaries,
	})
}

func (s *MCPServer) toolBookTransitTicket(ctx context.Context, id interface{}, args map[string]interface{}) {
	var req models.BookTicketRequest

	// If raw_payload was passed directly
	if raw, ok := args["raw_payload"].(map[string]interface{}); ok && len(raw) > 0 {
		b, err := json.Marshal(raw)
		if err == nil {
			_ = json.Unmarshal(b, &req)
		}
	}

	// Override or fill individual fields if provided
	if itID, ok := args["itineraries_id"].(string); ok && itID != "" {
		req.ItinerariesID = itID
	}
	if amt, ok := args["amount"].(string); ok && amt != "" {
		req.Amount = amt
	}
	if oLabel, ok := args["origin_label"].(string); ok && oLabel != "" {
		req.OriginLabel = oLabel
	}
	if dLabel, ok := args["destination_label"].(string); ok && dLabel != "" {
		req.DestinationLabel = dLabel
	}
	if ch, ok := args["payment_channel"].(string); ok && ch != "" {
		req.PaymentChannel = ch
	} else if req.PaymentChannel == "" {
		req.PaymentChannel = jaklingko.PaymentChannelQRIS
	}

	if sIDs, ok := args["service_ids"].([]interface{}); ok {
		var arr []string
		for _, v := range sIDs {
			arr = append(arr, fmt.Sprintf("%v", v))
		}
		req.ServiceIDs = arr
	}

	if req.ItinerariesID == "" {
		s.sendToolError(id, "Parameter 'itineraries_id' or 'raw_payload' is required")
		return
	}

	res, err := s.client.BookTicket(ctx, req)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Booking failed: %v", err))
		return
	}

	s.sendToolJSON(id, res)
}

func (s *MCPServer) toolGetPendingTickets(ctx context.Context, id interface{}) {
	tickets, err := s.client.GetPendingTickets(ctx)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to fetch pending tickets: %v", err))
		return
	}
	s.sendToolJSON(id, tickets)
}

func (s *MCPServer) toolGetActiveTickets(ctx context.Context, id interface{}, args map[string]interface{}) {
	includeQR, _ := args["include_qr"].(bool)

	tickets, err := s.client.GetActiveTickets(ctx)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to fetch active tickets: %v", err))
		return
	}

	if !includeQR {
		s.sendToolJSON(id, tickets)
		return
	}

	type FullTicketInfo struct {
		Summary models.ActiveTicketItem  `json:"summary"`
		Detail  *models.TicketDetailData `json:"detail,omitempty"`
		GateQRs []models.GateLegQRInfo   `json:"gate_qrs,omitempty"`
	}

	var results []FullTicketInfo
	for _, t := range tickets {
		info := FullTicketInfo{Summary: t}
		qrs, err := s.client.GenerateGateQRForTicket(ctx, t.TicketLongID, 0)
		if err == nil {
			info.GateQRs = qrs
		}
		detail, err := s.client.GetTicketDetail(ctx, t.TicketLongID)
		if err == nil {
			info.Detail = detail
		}
		results = append(results, info)
	}

	s.sendToolJSON(id, results)
}

func (s *MCPServer) toolGetTicketDetail(ctx context.Context, id interface{}, args map[string]interface{}) {
	ticketLongID, _ := args["ticket_long_id"].(string)
	if strings.TrimSpace(ticketLongID) == "" {
		s.sendToolError(id, "Parameter 'ticket_long_id' is required")
		return
	}

	detail, err := s.client.GetTicketDetail(ctx, ticketLongID)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to fetch ticket detail for '%s': %v", ticketLongID, err))
		return
	}

	s.sendToolJSON(id, detail)
}

func (s *MCPServer) toolGenerateGateQR(ctx context.Context, id interface{}, args map[string]interface{}) {
	ticketLongID, _ := args["ticket_long_id"].(string)
	if strings.TrimSpace(ticketLongID) == "" {
		s.sendToolError(id, "Parameter 'ticket_long_id' is required")
		return
	}

	legID := 0
	if l, ok := args["leg_id"].(float64); ok && l > 0 {
		legID = int(l)
	}

	qrs, err := s.client.GenerateGateQRForTicket(ctx, ticketLongID, legID)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to generate gate QR for ticket '%s': %v", ticketLongID, err))
		return
	}

	s.sendToolJSON(id, qrs)
}

// resolveTargetArg resolves ticket_long_id/leg_id arguments into a TapTarget,
// sending a tool error and returning false when that is not possible.
func (s *MCPServer) resolveTargetArg(ctx context.Context, id interface{}, args map[string]interface{}) (*jaklingko.TapTarget, bool) {
	ticketLongID, _ := args["ticket_long_id"].(string)
	if strings.TrimSpace(ticketLongID) == "" {
		s.sendToolError(id, "Parameter 'ticket_long_id' is required")
		return nil, false
	}

	legID := 0
	if l, ok := args["leg_id"].(float64); ok && l > 0 {
		legID = int(l)
	}

	target, err := s.client.ResolveTapTarget(ctx, ticketLongID, legID)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to resolve ticket '%s': %v", ticketLongID, err))
		return nil, false
	}
	return target, true
}

func (s *MCPServer) toolRefreshGateQR(ctx context.Context, id interface{}, args map[string]interface{}) {
	target, ok := s.resolveTargetArg(ctx, id, args)
	if !ok {
		return
	}

	qr, err := s.client.RefreshGateQR(ctx, target.TicketID)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to refresh gate QR: %v", err))
		return
	}

	s.sendToolJSON(id, map[string]interface{}{
		"ticket_id": target.TicketID,
		"legs_id":   target.LegsID,
		"direction": target.Direction,
		"qr":        qr,
	})
}

func (s *MCPServer) toolValidateGateQR(ctx context.Context, id interface{}, args map[string]interface{}) {
	qrString, _ := args["qr_string"].(string)
	if strings.TrimSpace(qrString) == "" {
		s.sendToolError(id, "Parameter 'qr_string' is required")
		return
	}

	target, ok := s.resolveTargetArg(ctx, id, args)
	if !ok {
		return
	}

	res, err := s.client.ValidateGateQR(ctx, target.TicketID, target.LegsID, qrString)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Gate QR validation failed: %v", err))
		return
	}

	s.sendToolJSON(id, map[string]interface{}{
		"ticket_id": target.TicketID,
		"legs_id":   target.LegsID,
		"result":    res.Result,
		"direction": res.Direction,
	})
}

func (s *MCPServer) toolWatchGateQR(ctx context.Context, id interface{}, args map[string]interface{}) {
	target, ok := s.resolveTargetArg(ctx, id, args)
	if !ok {
		return
	}

	intervalSeconds := 20.0
	if v, ok := args["interval_seconds"].(float64); ok && v > 0 {
		intervalSeconds = v
	}

	maxCycles := 1
	if m, ok := args["max_cycles"].(float64); ok && m > 0 {
		maxCycles = int(m)
	}
	if maxCycles > 60 {
		maxCycles = 60
	}

	var frames []jaklingko.QRFrame
	err := s.client.WatchGateQR(ctx, target.TicketID, target.LegsID, &jaklingko.WatchOptions{
		Interval:  time.Duration(intervalSeconds * float64(time.Second)),
		MaxCycles: maxCycles,
		OnFrame: func(f jaklingko.QRFrame) {
			frames = append(frames, f)
		},
	})
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Gate QR watch failed: %v", err))
		return
	}

	s.sendToolJSON(id, map[string]interface{}{
		"ticket_id":        target.TicketID,
		"legs_id":          target.LegsID,
		"direction":        target.Direction,
		"interval_seconds": intervalSeconds,
		"frames":           frames,
	})
}

func (s *MCPServer) toolGetPaymentChannels(ctx context.Context, id interface{}) {
	channels, err := s.client.GetPaymentChannels(ctx)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to fetch payment channels: %v", err))
		return
	}
	s.sendToolJSON(id, channels)
}

func (s *MCPServer) toolPayFelloTicket(ctx context.Context, id interface{}, args map[string]interface{}) {
	felloURL, _ := args["fello_url"].(string)
	pin, _ := args["pin"].(string)

	if strings.TrimSpace(pin) == "" {
		pin = os.Getenv("JAKLINGKO_FELLO_PIN")
		if pin == "" {
			pin = os.Getenv("FELLO_PIN")
		}
	}

	if strings.TrimSpace(felloURL) == "" {
		s.sendToolError(id, "Parameter 'fello_url' is required")
		return
	}
	if strings.TrimSpace(pin) == "" {
		s.sendToolError(id, "Fello PIN is required: provide 'pin' parameter or set JAKLINGKO_FELLO_PIN environment variable")
		return
	}

	if err := s.client.PayFelloTicket(ctx, felloURL, pin); err != nil {
		s.sendToolError(id, fmt.Sprintf("Fello payment failed: %v", err))
		return
	}

	s.sendToolJSON(id, map[string]interface{}{
		"status":  "SUCCESS",
		"message": "Fello payment successfully completed and verified. Ticket is now active.",
	})
}

func (s *MCPServer) toolGetBalance(ctx context.Context, id interface{}, args map[string]interface{}) {
	chID, _ := args["channel_id"].(string)
	if strings.TrimSpace(chID) != "" {
		bal, err := s.client.GetChannelBalance(ctx, chID)
		if err != nil {
			s.sendToolError(id, fmt.Sprintf("Failed to fetch balance for channel %s: %v", chID, err))
			return
		}
		s.sendToolJSON(id, bal)
		return
	}

	balances, err := s.client.GetAllBalances(ctx)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to fetch e-wallet balances: %v", err))
		return
	}
	s.sendToolJSON(id, balances)
}

// Helpers for JSON-RPC 2.0 output
func (s *MCPServer) sendResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	b, _ := json.Marshal(resp)
	fmt.Println(string(b))
}

func (s *MCPServer) sendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	b, _ := json.Marshal(resp)
	fmt.Println(string(b))
}

func (s *MCPServer) toolGetProfileByPhone(ctx context.Context, id interface{}, args map[string]interface{}) {
	phone, _ := args["phone"].(string)
	if strings.TrimSpace(phone) == "" {
		s.sendToolError(id, "Parameter 'phone' is required")
		return
	}

	profile, err := s.client.GetProfileByPhone(ctx, phone)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to look up user by phone '%s': %v", phone, err))
		return
	}

	s.sendToolJSON(id, profile)
}

func (s *MCPServer) toolTransferTicket(ctx context.Context, id interface{}, args map[string]interface{}) {
	ticketLongID, _ := args["ticket_long_id"].(string)
	phone, _ := args["phone"].(string)
	pin, _ := args["pin"].(string)
	confirm, _ := args["confirm"].(bool)

	if strings.TrimSpace(ticketLongID) == "" {
		s.sendToolError(id, "Parameter 'ticket_long_id' is required")
		return
	}
	if strings.TrimSpace(phone) == "" {
		s.sendToolError(id, "Parameter 'phone' is required")
		return
	}
	if !confirm {
		s.sendToolError(id, "Refusing to transfer without confirm=true: this action is irreversible")
		return
	}

	if strings.TrimSpace(pin) == "" {
		pin = os.Getenv("JAKLINGKO_PIN")
	}
	if strings.TrimSpace(pin) == "" {
		s.sendToolError(id, "A 6-digit account PIN is required: pass 'pin' or set JAKLINGKO_PIN")
		return
	}

	result, err := s.client.TransferTicketToPhone(ctx, ticketLongID, phone, pin)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Ticket transfer failed for '%s': %v", ticketLongID, err))
		return
	}

	s.sendToolJSON(id, result)
}

func (s *MCPServer) sendToolJSON(id interface{}, data interface{}) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		s.sendToolError(id, "Failed to serialize response: "+err.Error())
		return
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: MCPToolResult{
			Content: []MCPTextContent{
				{
					Type: "text",
					Text: string(b),
				},
			},
		},
	}
	respBytes, _ := json.Marshal(resp)
	fmt.Println(string(respBytes))
}

func (s *MCPServer) sendToolError(id interface{}, errMsg string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: MCPToolResult{
			Content: []MCPTextContent{
				{
					Type: "text",
					Text: "Error: " + errMsg,
				},
			},
			IsError: true,
		},
	}
	respBytes, _ := json.Marshal(resp)
	fmt.Println(string(respBytes))
}
