package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

func main() {
	versionFlag := flag.Bool("version", false, "Print version information and exit")
	usernameFlag := flag.String("user", "", "JakLingko registered phone/username (or set JAKLINGKO_USER)")
	passwordFlag := flag.String("pass", "", "JakLingko account password (or set JAKLINGKO_PASS)")
	originQuery := flag.String("origin", "Dukuh Atas", "Origin station or location name")
	destQuery := flag.String("destination", "Monas", "Destination station or location name")
	sessionFile := flag.String("session", jaklingko.DefaultSessionFile, "Path to session token file")
	logoutFlag := flag.Bool("logout", false, "Clear saved session file and exit")
	formatFlag := flag.String("format", "text", "Output format: text or json (for automation/tool parsing)")
	bookOption := flag.Int("book-option", 0, "Book ticket directly for the specified route option number (e.g. -book-option 1)")
	dumpPayload := flag.Int("dump-payload", 0, "Dump the ready-to-use booking payload JSON for the specified route option number")
	bookPayload := flag.String("book-payload", "", "Book ticket directly using a JSON payload string, file path, or stdin ('-')")
	pendingFlag := flag.Bool("pending", false, "List pending invoices awaiting payment")
	activeFlag := flag.Bool("active", false, "List active paid transit tickets")
	ticketDetailFlag := flag.String("ticket-detail", "", "View detailed itinerary, stations, and legs for a specific ticket (e.g. -ticket-detail T-DBNEP6...)")
	gateQRFlag := flag.String("gate-qr", "", "Generate turnstile gate tap-in QR for a specific ticket (e.g. -gate-qr T-DBNEP6...)")
	legIDFlag := flag.Int("leg-id", 0, "Specific transit leg ID for gate QR generation (optional, default: all transit legs)")
	allQRFlag := flag.Bool("all-qr", false, "Also generate turnstile gate QRs for all tickets when listing active tickets (-active)")
	paymentChannel := flag.String("channel", jaklingko.PaymentChannelQRIS, "Payment channel ID (default: 4 for QRIS)")
	baseURLFlag := flag.String("base-url", "", "Override API base URL (or set JAKLINGKO_BASE_URL)")
	cipherSaltFlag := flag.String("cipher-salt", "", "Override payload encryption salt (or set JAKLINGKO_CIPHER_SALT)")
	detailSaltFlag := flag.String("detail-salt", "", "Override ticket detail encryption salt (or set JAKLINGKO_DETAIL_SALT)")
	flag.Parse()

	isJSON := strings.ToLower(*formatFlag) == "json"

	if *versionFlag {
		if isJSON {
			fmt.Printf("{\"version\":\"%s\",\"commit\":\"%s\",\"date\":\"%s\"}\n", version, commit, date)
		} else {
			fmt.Printf("jaklingko-cli %s (commit: %s, built: %s)\n", version, commit, date)
		}
		return
	}

	// Helper for log messages: send to stderr in JSON mode so stdout remains pure standard JSON
	logMsg := func(format string, a ...interface{}) {
		if isJSON {
			fmt.Fprintf(os.Stderr, format+"\n", a...)
		} else {
			fmt.Printf(format+"\n", a...)
		}
	}

	if *logoutFlag {
		if err := jaklingko.ClearSession(*sessionFile); err != nil {
			logMsg("[ERROR] Failed to clear session: %v", err)
			os.Exit(1)
		}
		logMsg("[OK] Session successfully cleared.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !isJSON {
		fmt.Println("==================================================")
		fmt.Println("         JakLingko Transit & Fare Client          ")
		fmt.Println("==================================================")
	}

	// Initialize isolated client instance
	opts := []jaklingko.Option{jaklingko.WithSessionFile(*sessionFile)}
	if *baseURLFlag != "" {
		opts = append(opts, jaklingko.WithBaseURL(*baseURLFlag))
	}
	if *cipherSaltFlag != "" {
		opts = append(opts, jaklingko.WithCipherSalt(*cipherSaltFlag))
	}
	if *detailSaltFlag != "" {
		opts = append(opts, jaklingko.WithDetailSalt(*detailSaltFlag))
	}
	c := jaklingko.New(opts...)

	forceLogin := *usernameFlag != "" && *passwordFlag != ""
	if c.HasValidSession() && !forceLogin {
		sess := c.GetSession()
		logMsg("[OK] Using saved active session: %s (Saved at: %s)",
			sess.Username, sess.SavedAt.Format("2006-01-02 15:04:05"))
	} else {
		username := *usernameFlag
		if username == "" {
			username = os.Getenv("JAKLINGKO_USER")
			if username == "" {
				username = os.Getenv("JAKLINGKO_PHONE")
			}
		}
		password := *passwordFlag
		if password == "" {
			password = os.Getenv("JAKLINGKO_PASS")
			if password == "" {
				password = os.Getenv("JAKLINGKO_PASSWORD")
			}
		}

		if username == "" || password == "" {
			reader := bufio.NewReader(os.Stdin)
			if username == "" {
				fmt.Print("Enter phone number / username: ")
				input, _ := reader.ReadString('\n')
				username = strings.TrimSpace(input)
			}
			if password == "" {
				fmt.Print("Enter password: ")
				input, _ := reader.ReadString('\n')
				password = strings.TrimSpace(input)
			}
		}

		if username == "" || password == "" {
			logMsg("[ERROR] Username and password are required to log in!")
			os.Exit(1)
		}

		logMsg("\n[1/3] Authenticating user %s...", username)
		_, err := c.Login(ctx, username, password)
		if err != nil {
			logMsg("[FAIL] Login failed: %v", err)
			os.Exit(1)
		}

		logMsg("[OK] Authentication successful! Session saved to %s", *sessionFile)
	}

	// Mode A: Direct Book from JSON payload string, file, or stdin
	if *bookPayload != "" {
		handleBookPayload(ctx, c, *bookPayload, isJSON, logMsg)
		return
	}

	// Mode B: Check Pending Invoices
	if *pendingFlag {
		handlePendingTickets(ctx, c, isJSON, logMsg)
		return
	}

	// Mode C: View Ticket Detail
	if *ticketDetailFlag != "" {
		handleTicketDetail(ctx, c, *ticketDetailFlag, isJSON, logMsg)
		return
	}

	// Mode D: Generate Turnstile Gate QR
	if *gateQRFlag != "" {
		handleGateQR(ctx, c, *gateQRFlag, *legIDFlag, isJSON, logMsg)
		return
	}

	// Mode E: Check Active Paid Tickets
	if *activeFlag {
		handleActiveTickets(ctx, c, *allQRFlag, isJSON, logMsg)
		return
	}

	// Mode D: Search Transit Route & Calculate Fares
	logMsg("\n[2/3] Searching transit locations...")
	logMsg(" -> Searching Origin: '%s'...", *originQuery)
	originStops, err := c.SearchLocation(ctx, *originQuery, 5)
	if err != nil || len(originStops) == 0 {
		logMsg("[FAIL] Origin location '%s' not found: %v", *originQuery, err)
		os.Exit(1)
	}
	origin := originStops[0]
	logMsg("[OK] Origin resolved: %s (%s) [%.5f, %.5f]",
		origin.GetDisplayName(), origin.GetPTOName(), origin.GetLat(), origin.GetLng())

	logMsg(" -> Searching Destination: '%s'...", *destQuery)
	destStops, err := c.SearchLocation(ctx, *destQuery, 5)
	if err != nil || len(destStops) == 0 {
		logMsg("[FAIL] Destination location '%s' not found: %v", *destQuery, err)
		os.Exit(1)
	}
	destination := destStops[0]
	logMsg("[OK] Destination resolved: %s (%s) [%.5f, %.5f]",
		destination.GetDisplayName(), destination.GetPTOName(), destination.GetLat(), destination.GetLng())

	logMsg("\n[3/3] Calculating integrated transit fares...")
	fares, err := c.SearchTicketFare(ctx, origin, destination, nil)
	if err != nil {
		logMsg("[FAIL] Failed to calculate route fares: %v", err)
		os.Exit(1)
	}

	if len(fares) == 0 {
		logMsg("No transit route options found between '%s' and '%s'.", origin.GetDisplayName(), destination.GetDisplayName())
		return
	}

	// Mode D.1: Dump Booking Payload JSON for selected route option
	if *dumpPayload > 0 {
		if *dumpPayload > len(fares) {
			logMsg("[ERROR] Option number %d out of range (total options available: %d)", *dumpPayload, len(fares))
			os.Exit(1)
		}
		chosenFare := fares[*dumpPayload-1]
		payload := chosenFare.BookingPayload
		if payload == nil {
			p := jaklingko.BuildBookingPayload(chosenFare, *paymentChannel)
			payload = &p
		}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			logMsg("[ERROR] Failed to marshal payload: %v", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}

	// Mode D.2: Direct Booking for selected route option
	if *bookOption > 0 {
		if *bookOption > len(fares) {
			logMsg("[ERROR] Option number %d out of range (total options available: %d)", *bookOption, len(fares))
			os.Exit(1)
		}
		chosenFare := fares[*bookOption-1]
		payload := chosenFare.BookingPayload
		if payload == nil {
			p := jaklingko.BuildBookingPayload(chosenFare, *paymentChannel)
			payload = &p
		} else if *paymentChannel != "" && payload.PaymentChannel != *paymentChannel {
			payload.PaymentChannel = *paymentChannel
		}

		logMsg("\n[Action] Booking ticket for Option #%d (Total Fare: Rp %d)...", *bookOption, chosenFare.PTOCost)
		bookingRes, err := c.BookTicket(ctx, *payload)
		if err != nil {
			logMsg("[FAIL] Ticket booking failed: %v", err)
			if strings.Contains(err.Error(), "500") {
				logMsg("\nHint: JakLingko server stores route itineraries in Redis for ~3-5 minutes.")
				logMsg("If expired, please re-run the search to obtain a fresh itinerary ID.")
			}
			os.Exit(1)
		}

		printBookingResult(bookingRes, isJSON)
		return
	}

	// Output Route Options in JSON format
	if isJSON {
		b, err := json.MarshalIndent(fares, "", "  ")
		if err != nil {
			logMsg("[ERROR] Failed to format JSON output: %v", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}

	// Output Route Options in Clean Text format
	fmt.Printf("\n=== Integrated Transit Route Options (Found %d) ===\n\n", len(fares))
	for i, f := range fares {
		cost := f.PTOCost
		if cost <= 0 {
			cost = f.TotalAmount
		}

		fmt.Printf("[Option %d] Total Fare: Rp %d (Duration: %s, Distance: %.1f km)\n",
			i+1, cost, f.GeneralDuration, f.GeneralDistance)
		fmt.Printf("   Origin      : %s\n", f.Origin.GetDisplayName())
		fmt.Printf("   Destination : %s\n", f.Destination.GetDisplayName())
		fmt.Printf("   Itinerary ID: %s\n", f.ID)
		fmt.Println("   Transit Segments:")

		for j, leg := range f.Legs {
			operator := leg.ShowMode
			if operator == "" && leg.Agency != nil {
				operator = leg.Agency.Name
			}
			if operator == "" {
				operator = leg.Mode
			}
			lineInfo := ""
			if leg.Line != nil && leg.Line.Name != "" {
				lineInfo = fmt.Sprintf(" [%s]", leg.Line.Name)
			}
			fmt.Printf("     (%d) %-14s: %s -> %s%s\n",
				j+1, operator, leg.Origin.GetDisplayName(), leg.Destination.GetDisplayName(), lineInfo)
		}

		fmt.Printf("\n   To book this option, run:\n")
		fmt.Printf("   ./bin/jaklingko-cli -origin \"%s\" -destination \"%s\" -book-option %d\n",
			*originQuery, *destQuery, i+1)
		fmt.Println("--------------------------------------------------")
	}
}

func handleBookPayload(ctx context.Context, c jaklingko.Client, payloadInput string, isJSON bool, logMsg func(string, ...interface{})) {
	var rawJSON []byte
	var err error

	if payloadInput == "-" {
		logMsg("[Input] Reading booking payload JSON from stdin...")
		rawJSON, err = io.ReadAll(os.Stdin)
		if err != nil {
			logMsg("[ERROR] Failed to read from stdin: %v", err)
			os.Exit(1)
		}
	} else if strings.HasPrefix(strings.TrimSpace(payloadInput), "{") {
		rawJSON = []byte(payloadInput)
	} else {
		logMsg("[Input] Reading payload file from: %s", payloadInput)
		rawJSON, err = os.ReadFile(payloadInput)
		if err != nil {
			logMsg("[ERROR] Failed to read payload file %s: %v", payloadInput, err)
			os.Exit(1)
		}
	}

	var req models.BookTicketRequest
	if err := json.Unmarshal(rawJSON, &req); err != nil {
		logMsg("[ERROR] Failed to parse booking payload JSON: %v", err)
		os.Exit(1)
	}

	logMsg("[Action] Booking ticket for Itinerary ID: %s (Amount: Rp %s)...", req.ItinerariesID, req.Amount)
	res, err := c.BookTicket(ctx, req)
	if err != nil {
		logMsg("[FAIL] Booking failed: %v", err)
		if strings.Contains(err.Error(), "500") {
			logMsg("\nHint: JakLingko server stores route itineraries in Redis for ~3-5 minutes.")
			logMsg("If expired, please re-run the search to obtain a fresh itinerary ID.")
		}
		os.Exit(1)
	}

	printBookingResult(res, isJSON)
}

func handlePendingTickets(ctx context.Context, c jaklingko.Client, isJSON bool, logMsg func(string, ...interface{})) {
	logMsg("[Pending] Fetching pending tickets awaiting payment...")
	tickets, err := c.GetPendingTickets(ctx)
	if err != nil {
		logMsg("[FAIL] Failed to fetch pending tickets: %v", err)
		os.Exit(1)
	}

	if isJSON {
		b, _ := json.MarshalIndent(tickets, "", "  ")
		fmt.Println(string(b))
		return
	}

	if len(tickets) == 0 {
		fmt.Println("No pending tickets awaiting payment.")
		return
	}

	fmt.Printf("\n=== Pending Invoices Awaiting Payment (%d Found) ===\n\n", len(tickets))
	for i, t := range tickets {
		fmt.Printf("[%d] Invoice : %s\n", i+1, t.InvoiceID)
		fmt.Printf("    Route   : %s -> %s\n", t.TicketOriginLabel, t.TicketDestinationLabel)
		fmt.Printf("    Fare    : Rp %s (%s)\n", t.Amount, t.PaymentMethodName)
		fmt.Printf("    Status  : %s (%s)\n", t.Status, t.Date)
		fmt.Printf("    Long ID : %s\n", t.TicketLongID)
		fmt.Println("--------------------------------------------------")
	}
}

func handleActiveTickets(ctx context.Context, c jaklingko.Client, allQR bool, isJSON bool, logMsg func(string, ...interface{})) {
	logMsg("[Active] Fetching active transit tickets...")
	tickets, err := c.GetActiveTickets(ctx)
	if err != nil {
		logMsg("[FAIL] Failed to fetch active tickets: %v", err)
		os.Exit(1)
	}

	if len(tickets) == 0 {
		if isJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No active paid tickets found.")
		}
		return
	}

	if !allQR {
		if isJSON {
			b, _ := json.MarshalIndent(tickets, "", "  ")
			fmt.Println(string(b))
			return
		}

		fmt.Printf("\n=== Active Transit Tickets (%d Found) ===\n\n", len(tickets))
		for i, s := range tickets {
			fmt.Printf("[%d] Ticket ID      : %s\n", i+1, s.TicketLongID)
			fmt.Printf("    Status         : %s (Valid until %s)\n", strings.ToUpper(s.Status), s.ExpiredDate)
			fmt.Printf("    Route Journey  : %s -> %s\n", s.OriginLabel, s.DestinationLabel)
			if s.Distance != "" || s.Duration != "" {
				fmt.Printf("    Distance / Time: %s / %s\n", s.Distance, s.Duration)
			}
			fmt.Printf("    Payment Method : %s\n", s.PaymentMethodName)
			inJ := "No"
			if s.InJourney {
				inJ = "Yes (In Transit)"
			}
			fmt.Printf("    In Journey     : %s\n", inJ)
			fmt.Println("--------------------------------------------------")
		}

		fmt.Println("Tips:")
		fmt.Printf("  - View full legs & stations : jaklingko-cli -ticket-detail %s\n", tickets[0].TicketLongID)
		fmt.Printf("  - Generate turnstile gate QR: jaklingko-cli -gate-qr %s\n", tickets[0].TicketLongID)
		fmt.Println("  - Generate all QRs in bulk  : jaklingko-cli -active -all-qr")
		return
	}

	// allQR mode: fetch details and generate QRs for all tickets
	type FullTicketInfo struct {
		Summary models.ActiveTicketItem  `json:"summary"`
		Detail  *models.TicketDetailData `json:"detail,omitempty"`
		GateQRs []models.GateLegQRInfo   `json:"gate_qrs,omitempty"`
	}

	var results []FullTicketInfo
	for _, t := range tickets {
		info := FullTicketInfo{Summary: t}
		qrs, err := c.GenerateGateQRForTicket(ctx, t.TicketLongID, 0)
		if err == nil {
			info.GateQRs = qrs
		}
		detail, err := c.GetTicketDetail(ctx, t.TicketLongID)
		if err == nil {
			info.Detail = detail
		}
		results = append(results, info)
	}

	if isJSON {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(b))
		return
	}

	fmt.Printf("\n=== Active Transit Tickets & Turnstile Gate QRs (%d Found) ===\n\n", len(results))
	for i, item := range results {
		s := item.Summary
		fmt.Printf("[%d] Ticket ID      : %s\n", i+1, s.TicketLongID)
		fmt.Printf("    Status         : %s (Valid until %s)\n", strings.ToUpper(s.Status), s.ExpiredDate)
		fmt.Printf("    Route Journey  : %s -> %s\n", s.OriginLabel, s.DestinationLabel)
		fmt.Printf("    Payment Method : %s\n", s.PaymentMethodName)

		if item.Detail != nil {
			fmt.Println("    Transit Segments:")
			for j, leg := range item.Detail.Itineraries.Legs {
				op := leg.Agency.Name
				if op == "" && leg.Mode == "FOOT" {
					op = "Walk"
				}
				lineDesc := ""
				if leg.Line.Name != "" {
					lineDesc = fmt.Sprintf(" [%s]", leg.Line.Name)
				}
				fmt.Printf("      (%d) %-14s: %s -> %s%s\n",
					j+1, leg.Mode, leg.Origin.Name, leg.Destination.Name, lineDesc)
			}
		}

		if len(item.GateQRs) > 0 {
			fmt.Println("\n    --- Turnstile Gate QR (Tap-In Per Operator) ---")
			for _, gq := range item.GateQRs {
				fmt.Printf("    [%s] Leg #%d (%s -> %s):\n", gq.Operator, gq.LegIndex, gq.Origin, gq.Destination)
				if gq.Line != "" {
					fmt.Printf("      Route/Service : %s\n", gq.Line)
				}
				fmt.Printf("      QR String     : %s\n", gq.QR.QRString)
				fmt.Printf("      Valid Until   : %s\n", gq.QR.ExpiredDate)
				fmt.Println()
			}
		}
		fmt.Println("--------------------------------------------------")
	}
}

func handleTicketDetail(ctx context.Context, c jaklingko.Client, ticketLongID string, isJSON bool, logMsg func(string, ...interface{})) {
	logMsg("[Detail] Fetching details for ticket: %s...", ticketLongID)
	detail, err := c.GetTicketDetail(ctx, ticketLongID)
	if err != nil {
		logMsg("[FAIL] Failed to retrieve ticket detail: %v", err)
		os.Exit(1)
	}

	if isJSON {
		b, _ := json.MarshalIndent(detail, "", "  ")
		fmt.Println(string(b))
		return
	}

	t := detail.Ticket
	fmt.Printf("\n=== Ticket Detail: %s ===\n\n", t.TicketLongID)
	fmt.Printf("Ticket ID (Numeric) : %d\n", t.ID)
	fmt.Printf("Status              : %s\n", strings.ToUpper(t.Status))
	fmt.Printf("Journey Route       : %s -> %s\n", t.OriginLabel, t.DestinationLabel)
	if t.TotalCost > 0 {
		fmt.Printf("Total Fare          : Rp %d\n", t.TotalCost)
	}

	fmt.Printf("\n--- Transit Legs & Segments (%d Legs) ---\n\n", len(detail.Itineraries.Legs))
	for i, leg := range detail.Itineraries.Legs {
		op := leg.Agency.Name
		if op == "" {
			switch leg.Mode {
			case "TRANSIT_BUS":
				op = "TransJakarta"
			case "TRANSIT_TRAIN":
				op = "MRT Jakarta"
			case "FOOT":
				op = "Walking"
			default:
				op = leg.Mode
			}
		}

		fmt.Printf("[Leg #%d] %s (%s) [Leg ID: %d]\n", i+1, op, leg.Mode, leg.ID)
		origName := leg.Origin.GetDisplayName()
		if origName == "" {
			origName = leg.Origin.Name
		}
		destName := leg.Destination.GetDisplayName()
		if destName == "" {
			destName = leg.Destination.Name
		}
		fmt.Printf("  From     : %s\n", origName)
		fmt.Printf("  To       : %s\n", destName)
		if leg.Line.Name != "" {
			fmt.Printf("  Line     : %s\n", leg.Line.Name)
		}
		if leg.Fares > 0 {
			fmt.Printf("  Fare     : Rp %d\n", leg.Fares)
		}
		if leg.Distance > 0 || leg.Duration > 0 {
			fmt.Printf("  Distance : %d m (~%d mins)\n", leg.Distance, leg.Duration/60)
		}
		statusDesc := "Ready to Tap-In"
		if leg.IsPassed {
			statusDesc = "Completed / Passed"
		} else if leg.IsActive {
			statusDesc = "Currently Active (In Transit)"
		}
		fmt.Printf("  Status   : %s\n", statusDesc)
		fmt.Println()
	}

	fmt.Println("--------------------------------------------------")
	fmt.Println("To generate turnstile tap-in QR code:")
	fmt.Printf("  - All legs : jaklingko-cli -gate-qr %s\n", ticketLongID)
	for _, leg := range detail.Itineraries.Legs {
		if leg.Mode != "FOOT" {
			fmt.Printf("  - Leg ID %d : jaklingko-cli -gate-qr %s -leg-id %d\n", leg.ID, ticketLongID, leg.ID)
		}
	}
}

func handleGateQR(ctx context.Context, c jaklingko.Client, ticketLongID string, legID int, isJSON bool, logMsg func(string, ...interface{})) {
	logMsg("[Gate QR] Generating turnstile gate barcode for ticket %s (Leg ID: %d)...", ticketLongID, legID)
	qrs, err := c.GenerateGateQRForTicket(ctx, ticketLongID, legID)
	if err != nil {
		logMsg("[FAIL] Failed to generate turnstile gate QR: %v", err)
		os.Exit(1)
	}

	if isJSON {
		b, _ := json.MarshalIndent(qrs, "", "  ")
		fmt.Println(string(b))
		return
	}

	fmt.Printf("\n=== Turnstile Gate QR (%d Generated) ===\n\n", len(qrs))
	for _, gq := range qrs {
		fmt.Printf("[%d] Operator    : %s (Leg #%d, Leg ID: %d)\n", gq.LegIndex, gq.Operator, gq.LegIndex, gq.LegID)
		fmt.Printf("    Route       : %s -> %s\n", gq.Origin, gq.Destination)
		if gq.Line != "" {
			fmt.Printf("    Line/Route  : %s\n", gq.Line)
		}
		fmt.Printf("    QR String   : %s\n", gq.QR.QRString)
		fmt.Printf("    Valid Until : %s\n", gq.QR.ExpiredDate)
		fmt.Printf("    Gate Action : Tap-in at %s entry turnstile gate\n", gq.Operator)
		fmt.Println()
	}
	fmt.Println("--------------------------------------------------")
}

func printBookingResult(res *models.BookingResult, isJSON bool) {
	if isJSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
		return
	}

	fmt.Println("\n==================================================")
	fmt.Println("           Ticket Booking Successful!             ")
	fmt.Println("==================================================")
	fmt.Printf("Invoice ID        : %s\n", res.InvoiceID)
	fmt.Printf("Session ID        : %s\n", res.SessionID)
	fmt.Printf("Transaction Number: %s\n", res.TransactionNo)
	fmt.Printf("Payment Method    : %s\n", res.PaymentMethod)
	fmt.Printf("Total Bill        : Rp %s\n", res.Amount)
	if res.FeeAmount != "" && res.FeeAmount != "0" {
		fmt.Printf("Admin Fee         : Rp %s\n", res.FeeAmount)
	}
	fmt.Printf("Status            : %s\n", res.Status)

	if res.ValidityPeriod != "" {
		fmt.Printf("Payment Deadline  : %s\n", res.ValidityPeriod)
	}

	if res.QRWebURL != "" {
		fmt.Printf("Payment Web URL   : %s\n", res.QRWebURL)
	}

	if res.QRContent != "" {
		fmt.Println("\n--- EMVCo QRIS String (Copyable) ---")
		fmt.Println(res.QRContent)
	}
	fmt.Println("==================================================")
}
