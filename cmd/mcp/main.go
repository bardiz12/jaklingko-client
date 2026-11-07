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
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
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
		Name:        "get_payment_channels",
		Description: "List available payment methods supported by JakLingko (QRIS, OVO, Fello, etc.).",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
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
	case "get_payment_channels":
		s.toolGetPaymentChannels(ctx, req.ID)
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
			"origin":           origin.GetDisplayName(),
			"destination":      destination.GetDisplayName(),
			"total_found":      len(summaries),
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

func (s *MCPServer) toolGetPaymentChannels(ctx context.Context, id interface{}) {
	channels, err := s.client.GetPaymentChannels(ctx)
	if err != nil {
		s.sendToolError(id, fmt.Sprintf("Failed to fetch payment channels: %v", err))
		return
	}
	s.sendToolJSON(id, channels)
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
