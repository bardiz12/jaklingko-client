package jaklingko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bardiz12/jaklingko-client/pkg/crypto"
	"github.com/bardiz12/jaklingko-client/pkg/models"
)

// BuildBookingPayload constructs a ready-to-use BookTicketRequest from a selected RouteFare option
func BuildBookingPayload(fare models.RouteFare, channelID string) models.BookTicketRequest {
	if channelID == "" {
		channelID = PaymentChannelQRIS
	}

	amount := fare.PTOCost
	if amount <= 0 {
		amount = fare.TotalAmount
	}

	serviceIDs := fare.ServiceIDs
	if serviceIDs == nil {
		serviceIDs = []string{}
	}

	return models.BookTicketRequest{
		ItinerariesID:    fare.ID,
		PaymentChannel:   channelID,
		ServiceIDs:       serviceIDs,
		Amount:           fmt.Sprintf("%d", amount),
		OriginLabel:      fare.Origin.GetDisplayName(),
		DestinationLabel: fare.Destination.GetDisplayName(),
		Currency:         "360", // ISO 4217 code for IDR
		GrabCostDetails:  []interface{}{},
		Date:             time.Now().Format("15:04:05"),
		Lang:             "id",
	}
}

// BookTicket executes a ticket booking request using the provided BookTicketRequest payload.
// It encrypts the payload using AES-256-CBC PBKDF2 as required by JakLingko core API.
func (c *clientImpl) BookTicket(ctx context.Context, req models.BookTicketRequest) (*models.BookingResult, error) {
	if req.ItinerariesID == "" {
		return nil, fmt.Errorf("itinerariesId cannot be empty")
	}
	if req.PaymentChannel == "" {
		req.PaymentChannel = PaymentChannelQRIS
	}
	if req.Currency == "" {
		req.Currency = "360"
	}
	if req.Lang == "" {
		req.Lang = "id"
	}
	if req.Date == "" {
		req.Date = time.Now().Format("15:04:05")
	}
	if req.GrabCostDetails == nil {
		req.GrabCostDetails = []interface{}{}
	}

	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodPost, "ticket/v3/ticket-payment/add", req, true)
	if err != nil {
		return nil, fmt.Errorf("book ticket request failed: %w", err)
	}

	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return nil, fmt.Errorf("book ticket returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var apiRes models.ApiResult[models.TicketPaymentData]
	if err := json.Unmarshal(respBytes, &apiRes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal booking response: %w (raw: %s)", err, string(respBytes))
	}

	paymentData := apiRes.Data
	params := paymentData.Parameters

	paymentMethodName := "QRIS"
	if req.PaymentChannel != PaymentChannelQRIS {
		paymentMethodName = "Channel " + req.PaymentChannel
	}

	result := &models.BookingResult{
		InvoiceID:     params.GetInvoiceID(),
		SessionID:     params.SessionID,
		TransactionNo: params.TransactionNumber,
		Amount:        params.Amount,
		Status:        params.Status,
		PaymentMethod: paymentMethodName,
		QRWebURL:      paymentData.URL,
		PayloadUsed:   req,
	}

	if params.QRSnapData != nil {
		result.QRContent = params.QRSnapData.QRContent
		result.ValidityPeriod = params.QRSnapData.ValidityPeriod
		result.FeeAmount = params.QRSnapData.FeeAmount.Value
		if result.Amount == "" && params.QRSnapData.Amount.Value != "" {
			result.Amount = params.QRSnapData.Amount.Value
		}
	}

	return result, nil
}

// GetPaymentChannels fetches the list of available payment channels (QRIS, OVO, Fello, etc.)
func (c *clientImpl) GetPaymentChannels(ctx context.Context) ([]models.PaymentChannelInfo, error) {
	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodGet, "payment/v3/channels/list", nil, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment channels: %w", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("get payment channels returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var result models.ApiResult[[]models.PaymentChannelInfo]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payment channels: %w", err)
	}

	return result.Data, nil
}

// GetPendingTickets fetches tickets that are pending payment
func (c *clientImpl) GetPendingTickets(ctx context.Context) ([]models.PendingTicket, error) {
	endpoint := "ticket/v3/ticket-payment/list?page=1&view=20&lang=id"
	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodGet, endpoint, nil, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending tickets: %w", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("get pending tickets returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var result models.ApiResult[[]models.PendingTicket]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pending tickets: %w", err)
	}

	return result.Data, nil
}

// GetActiveTickets fetches paid transit tickets that are currently active
func (c *clientImpl) GetActiveTickets(ctx context.Context) ([]models.ActiveTicketItem, error) {
	endpoint := "ticket/v3/ticket/list?page=1&view=20&filter[type]=ACTIVE"
	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodGet, endpoint, nil, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get active tickets: %w", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("get active tickets returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var result models.ApiResult[[]models.ActiveTicketItem]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal active tickets: %w", err)
	}

	return result.Data, nil
}

// GetTicketDetail retrieves the full itinerary legs and details for an active ticket
func (c *clientImpl) GetTicketDetail(ctx context.Context, ticketLongID string) (*models.TicketDetailData, error) {
	// The JakLingko API requires ticket longId to be encrypted using the configured ticket detail salt
	ce := crypto.NewCipherEngine(c.detailSalt)
	encJSON, err := ce.EncryptPayload(ticketLongID)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt ticket id: %w", err)
	}

	var obj map[string]string
	if err := json.Unmarshal([]byte(encJSON), &obj); err != nil {
		return nil, fmt.Errorf("failed to parse encrypted ticket payload: %w", err)
	}
	rawCipher := obj["request"]

	endpoint := "ticket/v3/ticket/detail/" + rawCipher
	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodGet, endpoint, nil, false)
	if err != nil {
		return nil, fmt.Errorf("ticket detail request failed: %w", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("get ticket detail returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var result models.ApiResult[models.TicketDetailData]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ticket detail: %w", err)
	}

	return &result.Data, nil
}

// GenerateGateQR generates the turnstile barcode (JLI#...) for tapping in at transit stations
func (c *clientImpl) GenerateGateQR(ctx context.Context, ticketID, legsID int) (*models.GateQRResult, error) {
	payload := map[string]int{
		"ticket_id": ticketID,
		"legs_id":   legsID,
	}

	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodPost, "ticket/v3/qr-activation/generate", payload, true)
	if err != nil {
		return nil, fmt.Errorf("generate gate QR request failed: %w", err)
	}

	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return nil, fmt.Errorf("generate gate QR returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var result models.ApiResult[models.GateQRResult]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gate QR response: %w", err)
	}

	return &result.Data, nil
}

// GenerateGateQRForTicket resolves a ticket's integer ID from its ticket long ID and generates
// the turnstile gate barcode for a specific leg ID (or for all non-walking legs if legID <= 0).
func (c *clientImpl) GenerateGateQRForTicket(ctx context.Context, ticketLongID string, legID int) ([]models.GateLegQRInfo, error) {
	detail, err := c.GetTicketDetail(ctx, ticketLongID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ticket details for %s: %w", ticketLongID, err)
	}

	if detail == nil || detail.Ticket.ID == 0 {
		return nil, fmt.Errorf("invalid ticket detail data for %s: ticket integer ID missing", ticketLongID)
	}

	legs := detail.Itineraries.Legs
	if len(legs) == 0 {
		return nil, fmt.Errorf("no transit legs found for ticket %s", ticketLongID)
	}

	var results []models.GateLegQRInfo

	for idx, leg := range legs {
		if legID > 0 && leg.ID != legID {
			continue
		}
		if legID <= 0 && leg.Mode == "FOOT" {
			continue
		}

		qr, err := c.GenerateGateQR(ctx, detail.Ticket.ID, leg.ID)
		if err != nil {
			if legID > 0 {
				return nil, fmt.Errorf("failed to generate gate QR for leg %d: %w", leg.ID, err)
			}
			// For bulk all-legs generation, skip failing legs
			continue
		}

		op := leg.Agency.Name
		if op == "" {
			switch leg.Mode {
			case "TRANSIT_BUS":
				op = "TransJakarta"
			case "TRANSIT_TRAIN":
				op = "MRT Jakarta"
			default:
				op = leg.Mode
			}
		}

		origName := leg.Origin.GetDisplayName()
		if origName == "" {
			origName = leg.Origin.Name
		}
		destName := leg.Destination.GetDisplayName()
		if destName == "" {
			destName = leg.Destination.Name
		}

		results = append(results, models.GateLegQRInfo{
			LegIndex:    idx + 1,
			LegID:       leg.ID,
			Mode:        leg.Mode,
			Operator:    op,
			Line:        leg.Line.Name,
			Origin:      origName,
			Destination: destName,
			QR:          *qr,
		})
	}

	if len(results) == 0 {
		if legID > 0 {
			var avail []string
			for _, l := range legs {
				avail = append(avail, fmt.Sprintf("%d (%s)", l.ID, l.Mode))
			}
			return nil, fmt.Errorf("leg ID %d not found in ticket %s (available legs: %s)", legID, ticketLongID, strings.Join(avail, ", "))
		}
		return nil, fmt.Errorf("no eligible transit legs found to generate turnstile gate QR for ticket %s", ticketLongID)
	}

	return results, nil
}
