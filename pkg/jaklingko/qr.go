package jaklingko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bardiz12/jaklingko-client/pkg/models"
)

// DefaultQRRefreshInterval mirrors the Android client's gate QR lifetime.
// TicketQrFragment, ScanTicketActivity and ABTQRCodeActivity each start a
// CountDownTimer(20000L, 1000L) and request a fresh QR when it fires.
const DefaultQRRefreshInterval = 20 * time.Second

// RefreshGateQR requests a new gate QR for an already-activated ticket.
//
// The Android client does not call this endpoint in v1.6.22 (it re-invokes
// GenerateGateQR instead), but the route exists in its Retrofit interface and is
// exposed here for completeness.
func (c *clientImpl) RefreshGateQR(ctx context.Context, ticketID int) (*models.GateQRResult, error) {
	if ticketID <= 0 {
		return nil, fmt.Errorf("ticketID must be a positive integer")
	}

	payload := models.RefreshQRRequest{TicketID: ticketID}
	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodPost, "ticket/v3/qr-activation/refresh", payload, true)
	if err != nil {
		return nil, fmt.Errorf("refresh gate QR request failed: %w", err)
	}

	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return nil, fmt.Errorf("refresh gate QR returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var result models.ApiResult[models.GateQRResult]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gate QR response: %w", err)
	}

	return &result.Data, nil
}

// ValidateGateQR submits a scanned gate QR string to the backend. The response
// payload carries the tap direction: "CHECKIN" for tap-in, anything else for
// tap-out (the backend decides the direction from the ticket's journey state).
func (c *clientImpl) ValidateGateQR(ctx context.Context, ticketID, legID int, qrString string) (*models.GateQRValidation, error) {
	if ticketID <= 0 {
		return nil, fmt.Errorf("ticketID must be a positive integer")
	}
	if legID <= 0 {
		return nil, fmt.Errorf("legID must be a positive integer")
	}
	if strings.TrimSpace(qrString) == "" {
		return nil, fmt.Errorf("qrString cannot be empty")
	}

	payload := models.ValidateQRRequest{
		TicketID: ticketID,
		LegsID:   legID,
		QRString: qrString,
	}

	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodPost, "ticket/v3/qr-activation/validate", payload, true)
	if err != nil {
		return nil, fmt.Errorf("validate gate QR request failed: %w", err)
	}

	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		var apiErr models.ApiResult[any]
		if jsonErr := json.Unmarshal(respBytes, &apiErr); jsonErr == nil && len(apiErr.Error) > 0 {
			msg := strings.Join(apiErr.Error, ", ")
			if apiErr.Message != "" {
				msg = apiErr.Message + ": " + msg
			}
			return nil, fmt.Errorf("gate QR validation rejected: %s", msg)
		}
		return nil, fmt.Errorf("validate gate QR returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var result models.ApiResult[string]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gate QR validation response: %w", err)
	}

	return &models.GateQRValidation{
		Result:    result.Data,
		Direction: models.DirectionFromValidation(result.Data),
	}, nil
}

// WatchOptions configures WatchGateQR.
type WatchOptions struct {
	// Interval between QR regenerations. Defaults to DefaultQRRefreshInterval.
	Interval time.Duration
	// MaxCycles stops the loop after N emitted frames (0 = run until the
	// context is cancelled).
	MaxCycles int
	// OnFrame receives every emitted QR frame, including failures.
	OnFrame func(QRFrame)
}

// QRFrame is a single QR emission from WatchGateQR.
type QRFrame struct {
	// Cycle is the 1-based regeneration counter.
	Cycle int `json:"cycle"`
	// Generated is when the QR was requested.
	Generated time.Time `json:"generated"`
	// QR holds the generated barcode (nil when Err is set).
	QR *models.GateQRResult `json:"qr,omitempty"`
	// Err reports a failed regeneration.
	Err error `json:"-"`
}

// WatchGateQR regenerates the gate QR on a fixed interval, mirroring the
// mobile app's 20 second countdown. The first QR is generated immediately; each
// subsequent QR is requested after Interval elapses. The loop stops on context
// cancellation, after MaxCycles frames, or on the first generation error.
func (c *clientImpl) WatchGateQR(ctx context.Context, ticketID, legID int, opts *WatchOptions) error {
	if ticketID <= 0 {
		return fmt.Errorf("ticketID must be a positive integer")
	}
	if legID <= 0 {
		return fmt.Errorf("legID must be a positive integer")
	}

	interval := DefaultQRRefreshInterval
	maxCycles := 0
	var onFrame func(QRFrame)
	if opts != nil {
		if opts.Interval > 0 {
			interval = opts.Interval
		}
		maxCycles = opts.MaxCycles
		onFrame = opts.OnFrame
	}

	for cycle := 1; ; cycle++ {
		qr, err := c.GenerateGateQR(ctx, ticketID, legID)
		if onFrame != nil {
			onFrame(QRFrame{Cycle: cycle, Generated: time.Now(), QR: qr, Err: err})
		}
		if err != nil {
			return fmt.Errorf("gate QR cycle %d failed: %w", cycle, err)
		}
		if maxCycles > 0 && cycle >= maxCycles {
			return nil
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// TapTarget describes the ticket and leg pair to activate, plus the direction
// the ticket is currently expected to move in.
type TapTarget struct {
	TicketID    int                 `json:"ticket_id"`
	LegsID      int                 `json:"legs_id"`
	Direction   models.TapDirection `json:"direction"`
	Mode        string              `json:"mode,omitempty"`
	Origin      string              `json:"origin,omitempty"`
	Destination string              `json:"destination,omitempty"`
}

// ResolveTapTarget resolves a ticket long ID into the integer ticket/leg IDs
// required by the gate QR endpoints. When legID is 0 the leg is chosen from the
// ticket state: the next unpassed leg for a tap-in, or the leg currently in
// journey for a tap-out.
//
// The direction is advisory only; the backend decides tap-in vs tap-out when
// the QR is validated.
func (c *clientImpl) ResolveTapTarget(ctx context.Context, ticketLongID string, legID int) (*TapTarget, error) {
	if strings.TrimSpace(ticketLongID) == "" {
		return nil, fmt.Errorf("ticketLongID cannot be empty")
	}

	detail, err := c.GetTicketDetail(ctx, ticketLongID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ticket details for %s: %w", ticketLongID, err)
	}
	if detail == nil || detail.Ticket.ID == 0 {
		return nil, fmt.Errorf("invalid ticket detail data for %s: ticket integer ID missing", ticketLongID)
	}

	legs := detail.Itineraries.Legs
	if len(legs) == 0 {
		return nil, fmt.Errorf("no legs found for ticket %s", ticketLongID)
	}

	direction := models.TapDirectionIn
	if detail.Ticket.InJourney {
		direction = models.TapDirectionOut
	}

	var selected *models.TicketLegDetail
	if legID > 0 {
		for i := range legs {
			if legs[i].ID == legID {
				selected = &legs[i]
				break
			}
		}
		if selected == nil {
			var avail []string
			for _, l := range legs {
				avail = append(avail, fmt.Sprintf("%d (%s)", l.ID, l.Mode))
			}
			return nil, fmt.Errorf("leg ID %d not found in ticket %s (available legs: %s)", legID, ticketLongID, strings.Join(avail, ", "))
		}
	} else {
		selected = pickTapLeg(legs, direction)
		if selected == nil {
			return nil, fmt.Errorf("no eligible transit legs found for ticket %s", ticketLongID)
		}
	}

	return &TapTarget{
		TicketID:    detail.Ticket.ID,
		LegsID:      selected.ID,
		Direction:   direction,
		Mode:        selected.Mode,
		Origin:      selected.Origin.GetDisplayName(),
		Destination: selected.Destination.GetDisplayName(),
	}, nil
}

// pickTapLeg chooses the most relevant transit leg for the given direction.
func pickTapLeg(legs []models.TicketLegDetail, direction models.TapDirection) *models.TicketLegDetail {
	if direction == models.TapDirectionIn {
		// Tap-in targets the next leg that has not been completed yet.
		for i := range legs {
			if legs[i].Mode == "FOOT" || legs[i].IsPassed {
				continue
			}
			return &legs[i]
		}
		return nil
	}

	// Tap-out targets the leg currently in journey, falling back to the last
	// transit leg on the itinerary.
	for i := range legs {
		if legs[i].Mode == "FOOT" || legs[i].IsPassed {
			continue
		}
		if legs[i].IsActive {
			return &legs[i]
		}
	}

	var last *models.TicketLegDetail
	for i := range legs {
		if legs[i].Mode == "FOOT" {
			continue
		}
		last = &legs[i]
	}
	return last
}
