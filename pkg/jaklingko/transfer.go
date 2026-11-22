package jaklingko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bardiz12/jaklingko-client/pkg/models"
)

// GetProfileByPhone resolves a registered JakLingko user by phone number. This
// is the recipient lookup performed before a ticket transfer and returns the
// destination user ID required by the transfer endpoint.
func (c *clientImpl) GetProfileByPhone(ctx context.Context, phone string) (*models.UserByPhone, error) {
	normPhone := NormalizePhoneNumber(phone)
	if normPhone == "" {
		return nil, fmt.Errorf("phone cannot be empty")
	}

	endpoint := "auth/v3/profile/by-phone/" + normPhone
	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodGet, endpoint, nil, false)
	if err != nil {
		return nil, fmt.Errorf("profile lookup request failed: %w", err)
	}

	if statusCode != http.StatusOK {
		var apiErr models.StatusResult
		if jsonErr := json.Unmarshal(respBytes, &apiErr); jsonErr == nil {
			return nil, fmt.Errorf("profile lookup rejected: %s", formatStatus(apiErr))
		}
		return nil, fmt.Errorf("profile lookup returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var result models.ApiResult[models.UserByPhone]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal profile lookup response: %w (raw: %s)", err, string(respBytes))
	}
	if result.Data.UserID == 0 {
		return nil, fmt.Errorf("profile lookup returned no user for phone %s", normPhone)
	}

	return &result.Data, nil
}

// TransferTicket transfers a ticket to another JakLingko user. The body is
// encrypted, matching the app's interceptor behaviour. Success is signalled by
// a "SUCCESS" message rather than the HTTP status alone.
func (c *clientImpl) TransferTicket(ctx context.Context, req models.TransferTicketRequest) (*models.StatusResult, error) {
	req.PIN = strings.TrimSpace(req.PIN)
	req.TicketID = strings.TrimSpace(req.TicketID)
	req.Phone = strings.TrimSpace(req.Phone)

	if req.PIN == "" {
		return nil, fmt.Errorf("PIN is required to transfer a ticket")
	}
	if req.DestinationUID <= 0 {
		return nil, fmt.Errorf("destination user ID must be a positive integer")
	}
	if req.TicketID == "" {
		return nil, fmt.Errorf("ticket ID cannot be empty")
	}
	if req.Phone == "" {
		return nil, fmt.Errorf("recipient phone cannot be empty")
	}
	if req.Language == "" {
		req.Language = "id"
	}

	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodPost, "ticket/v3/ticket/transfer", req, true)
	if err != nil {
		return nil, fmt.Errorf("ticket transfer request failed: %w", err)
	}

	var result models.StatusResult
	unmarshalErr := json.Unmarshal(respBytes, &result)

	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		if unmarshalErr == nil {
			return nil, fmt.Errorf("ticket transfer rejected (HTTP %d): %s", statusCode, formatStatus(result))
		}
		return nil, fmt.Errorf("ticket transfer returned HTTP %d: %s", statusCode, string(respBytes))
	}

	if unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal ticket transfer response: %w (raw: %s)", unmarshalErr, string(respBytes))
	}

	if !strings.EqualFold(result.Message, models.TransferSuccessMessage) {
		return nil, fmt.Errorf("ticket transfer failed: %s", formatStatus(result))
	}

	return &result, nil
}

// TransferTicketToPhone resolves the recipient by phone number and transfers
// the ticket to that user.
func (c *clientImpl) TransferTicketToPhone(ctx context.Context, ticketLongID, phone, pin string) (*models.TransferResult, error) {
	recipient, err := c.GetProfileByPhone(ctx, phone)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve transfer recipient: %w", err)
	}
	return c.TransferTicketToProfile(ctx, ticketLongID, recipient, pin)
}

// TransferTicketToProfile transfers a ticket to an already resolved recipient.
// It rejects self-transfers and tickets that are already in use, mirroring the
// guards the Android app applies before allowing a transfer.
func (c *clientImpl) TransferTicketToProfile(ctx context.Context, ticketLongID string, recipient *models.UserByPhone, pin string) (*models.TransferResult, error) {
	ticketLongID = strings.TrimSpace(ticketLongID)
	if ticketLongID == "" {
		return nil, fmt.Errorf("ticket ID cannot be empty")
	}
	if recipient == nil || recipient.UserID <= 0 {
		return nil, fmt.Errorf("recipient profile is missing a valid user ID")
	}

	if sess := c.GetSession(); sess != nil && strings.TrimSpace(sess.Username) != "" &&
		NormalizePhoneNumber(sess.Username) == NormalizePhoneNumber(recipient.Phone) {
		return nil, fmt.Errorf("cannot transfer a ticket to your own account (%s)", recipient.Phone)
	}

	detail, err := c.GetTicketDetail(ctx, ticketLongID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ticket details for %s: %w", ticketLongID, err)
	}
	if detail != nil && (detail.Ticket.IsUsed || detail.Ticket.InJourney) {
		return nil, fmt.Errorf("ticket %s cannot be transferred because it is already in use", ticketLongID)
	}

	firstname, lastname := SplitRecipientName(recipient.Name)
	status, err := c.TransferTicket(ctx, models.TransferTicketRequest{
		PIN:            pin,
		DestinationUID: recipient.UserID,
		TicketID:       ticketLongID,
		Phone:          recipient.Phone,
		Firstname:      firstname,
		Lastname:       lastname,
		Language:       recipient.Lang,
	})
	if err != nil {
		return nil, err
	}

	return &models.TransferResult{
		TicketID:  ticketLongID,
		Recipient: *recipient,
		Status:    *status,
	}, nil
}

// SplitRecipientName splits a recipient's display name into the first and last
// name fields expected by the transfer endpoint. It mirrors the Android helper
// qa.d.x: the first token is the first name, the last token is the last name,
// and a single-token name yields an empty last name.
func SplitRecipientName(name string) (string, string) {
	fields := strings.Fields(strings.TrimSpace(name))
	switch len(fields) {
	case 0:
		return "", ""
	case 1:
		return fields[0], ""
	default:
		return fields[0], fields[len(fields)-1]
	}
}

// formatStatus renders a status envelope for error messages.
func formatStatus(status models.StatusResult) string {
	if len(status.Error) > 0 {
		msg := strings.Join(status.Error, ", ")
		if status.Message != "" {
			msg = status.Message + ": " + msg
		}
		return msg
	}
	if status.Message != "" {
		return status.Message
	}
	return fmt.Sprintf("status %d", status.StatusCode)
}
