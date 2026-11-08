package models

// BookTicketRequest represents the exact payload expected by ticket/v3/ticket-payment/add
type BookTicketRequest struct {
	ItinerariesID    string        `json:"itinerariesId"`
	PaymentChannel   string        `json:"paymentChannel"` // "4" for QRIS
	ServiceIDs       []string      `json:"serviceIds"`
	Amount           string        `json:"amount"`
	OriginLabel      string        `json:"origin_label"`
	DestinationLabel string        `json:"destination_label"`
	Currency         string        `json:"currency"`          // "360" for IDR
	GrabCostDetails  []interface{} `json:"grab_cost_details"` // empty array if no grab
	Date             string        `json:"date"`              // "HH:mm:ss"
	Lang             string        `json:"lang"`              // "id"
}

// PaymentChannelInfo represents a payment channel available in JakLingko (from payment/v3/channels/list)
type PaymentChannelInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Category      string `json:"category"`
	Enable        bool   `json:"enable"`
	StatusBinding bool   `json:"statusBinding"`
	PictureURL    string `json:"pictureUrl"`
}

// ChannelBalance represents the balance information returned by payment/v3/channels/balance/{id}
type ChannelBalance struct {
	Balance            string `json:"balance"`
	BalanceUnformatted string `json:"balance_unformated"`
}

// ChannelBalanceInfo combines channel details with balance info for user display
type ChannelBalanceInfo struct {
	ChannelID          string `json:"channel_id"`
	ChannelName        string `json:"channel_name"`
	Category           string `json:"category"`
	Bound              bool   `json:"bound"`
	Balance            string `json:"balance,omitempty"`
	BalanceUnformatted string `json:"balance_unformated,omitempty"`
	Error              string `json:"error,omitempty"`
}

// QRSnapAmount represents amount details in QRIS snap response
type QRSnapAmount struct {
	Currency string `json:"currency"`
	Value    string `json:"value"`
}

// QRSnapData represents the QRIS transaction data returned by the backend
type QRSnapData struct {
	QRContent           string       `json:"qrContent"`
	QRImage             string       `json:"qrImage"`
	QRURL               string       `json:"qrUrl"`
	RedirectURL         string       `json:"redirectUrl"`
	PartnerReferenceNo  string       `json:"partnerReferenceNo"`
	ReferenceNo         string       `json:"referenceNo"`
	MerchantID          string       `json:"merchantId"`
	Amount              QRSnapAmount `json:"amount"`
	FeeAmount           QRSnapAmount `json:"feeAmount"`
	ValidityPeriod      string       `json:"validityPeriod"`
	ResponseCode        string       `json:"responseCode"`
	ResponseDescription string       `json:"responseDescription"`
}

// PaymentParameters represents the parameters returned inside data
type PaymentParameters struct {
	Amount             string      `json:"Amount"`
	AmountAlt          string      `json:"amount"`
	InvoiceID          string      `json:"InvoiceID"`
	InvoiceIDAlt       string      `json:"invoiceID"`
	InvoiceReferenceID string      `json:"InvoiceReferenceId"`
	Status             string      `json:"Status"`
	TransactionNumber  string      `json:"TransactionNumber"`
	SessionID          string      `json:"sessionID"`
	QRSnapData         *QRSnapData `json:"qrSnapData,omitempty"`
}

// GetInvoiceID returns the non-empty invoice ID
func (p *PaymentParameters) GetInvoiceID() string {
	if p.InvoiceID != "" {
		return p.InvoiceID
	}
	return p.InvoiceIDAlt
}

// TicketPaymentData represents the data object inside ticket payment response
type TicketPaymentData struct {
	Headers    map[string]interface{} `json:"headers"`
	Parameters PaymentParameters      `json:"parameters"`
	URL        string                 `json:"url"`
}

// BookingResult is the user-friendly structured result of a ticket booking (ideal for MCP tool output)
type BookingResult struct {
	InvoiceID      string            `json:"invoice_id"`
	SessionID      string            `json:"session_id"`
	TransactionNo  string            `json:"transaction_number"`
	Amount         string            `json:"amount"`
	FeeAmount      string            `json:"fee_amount,omitempty"`
	Status         string            `json:"status"`
	PaymentMethod  string            `json:"payment_method"`
	QRContent      string            `json:"qr_content,omitempty"`
	QRWebURL       string            `json:"qr_web_url,omitempty"`
	ValidityPeriod string            `json:"validity_period,omitempty"`
	PayloadUsed    BookTicketRequest `json:"payload_used"`
}

// PendingTicket represents a pending ticket awaiting payment
type PendingTicket struct {
	InvoiceID              string `json:"invoice_id"`
	TraceNumber            string `json:"trace_number"`
	TicketLongID           string `json:"ticket_longid"`
	Amount                 string `json:"amount"`
	Date                   string `json:"date"`
	PaymentMethodCode      int    `json:"payment_method_code"`
	PaymentMethodName      string `json:"payment_method_name"`
	Status                 string `json:"status"`
	TicketOriginLabel      string `json:"ticket_origin_label"`
	TicketDestinationLabel string `json:"ticket_destination_label"`
}

// ActiveTicketItem represents an active transit ticket returned by ticket/v3/ticket/list
type ActiveTicketItem struct {
	TicketLongID      string `json:"ticket_longid"`
	OriginDate        string `json:"origin_date"`
	OriginLabel       string `json:"origin_label"`
	DestinationDate   string `json:"destination_date"`
	DestinationLabel  string `json:"destination_label"`
	Distance          string `json:"distance"`
	Duration          string `json:"duration"`
	ExpiredDate       string `json:"expired_date"`
	PaymentMethodName string `json:"payment_method_name"`
	Status            string `json:"status"`
	InJourney         bool   `json:"in_journey"`
}

// TicketLegDetail represents an individual leg of an active ticket
type TicketLegDetail struct {
	ID          int          `json:"id"`
	Mode        string       `json:"mode"`
	Distance    int          `json:"distance"`
	Duration    int          `json:"duration"`
	Fares       int          `json:"fares"`
	IsActive    bool         `json:"is_active"`
	IsPassed    bool         `json:"is_passed"`
	Agency      Agency       `json:"agency"`
	Line        TransitLine  `json:"line"`
	Origin      StopLocation `json:"origin"`
	Destination StopLocation `json:"destination"`
}

// TicketDetailData represents full ticket itinerary details from ticket/v3/ticket/detail
type TicketDetailData struct {
	Ticket struct {
		ID                int    `json:"id"`
		TicketLongID      string `json:"ticket_longid"`
		OriginLabel       string `json:"origin_label"`
		DestinationLabel  string `json:"destination_label"`
		TotalCost         int    `json:"total_cost"`
		Status            string `json:"label"`
		TicketIssuedDate  string `json:"ticket_issued_date"`
		TicketExpiredDate string `json:"ticket_expired_date"`
		PaymentMethodName string `json:"payment_method_name"`
		InJourney         bool   `json:"in_journey"`
		IsUsed            bool   `json:"is_used"`
	} `json:"ticket"`
	Itineraries struct {
		ID   string            `json:"id"`
		Legs []TicketLegDetail `json:"legs"`
	} `json:"itineraries"`
}

// GateQRResult represents the QR barcode string for tapping in at transit turnstiles
type GateQRResult struct {
	QRString    string `json:"qr_string"`
	ExpiredDate string `json:"expired_date"`
}

// GateLegQRInfo represents detailed gate QR information for a specific transit leg
type GateLegQRInfo struct {
	LegIndex    int          `json:"leg_index"`
	LegID       int          `json:"leg_id"`
	Mode        string       `json:"mode"`
	Operator    string       `json:"operator"`
	Line        string       `json:"line,omitempty"`
	Origin      string       `json:"origin"`
	Destination string       `json:"destination"`
	QR          GateQRResult `json:"qr"`
}
