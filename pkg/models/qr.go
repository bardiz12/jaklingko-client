package models

import "strings"

// Gate QR activation models for the JakLingko turnstile (tap-in / tap-out) flow.
//
// Static analysis reference (JakLingko Android v1.6.22):
//
//	POST ticket/v3/qr-activation/generate  {"legs_id":N,"ticket_id":N}                  -> {"qr_string":"JLI#..."}
//	POST ticket/v3/qr-activation/refresh   {"ticket_id":N}                              -> {"qr_string":"JLI#..."}
//	POST ticket/v3/qr-activation/validate  {"ticket_id":N,"legs_id":N,"qr_string":"..."} -> "CHECKIN" | ...
//
// The Android client regenerates its gate QR every 20 seconds by calling the
// generate endpoint again; the refresh endpoint is declared in its Retrofit
// interface but is not invoked anywhere in v1.6.22.

// GenerateQRRequest is the body for ticket/v3/qr-activation/generate.
type GenerateQRRequest struct {
	TicketID int `json:"ticket_id"`
	LegsID   int `json:"legs_id"`
}

// RefreshQRRequest is the body for ticket/v3/qr-activation/refresh.
// Unlike generate, it only identifies the ticket, not the leg.
type RefreshQRRequest struct {
	TicketID int `json:"ticket_id"`
}

// ValidateQRRequest is the body for ticket/v3/qr-activation/validate.
type ValidateQRRequest struct {
	TicketID int    `json:"ticket_id"`
	LegsID   int    `json:"legs_id"`
	QRString string `json:"qr_string"`
}

// TapDirection describes whether a gate QR is expected to tap a passenger in
// or out of the paid area.
type TapDirection string

const (
	// TapDirectionIn means the QR activates entry into the transit system.
	TapDirectionIn TapDirection = "TAP_IN"
	// TapDirectionOut means the QR activates exit from the transit system.
	TapDirectionOut TapDirection = "TAP_OUT"
	// TapDirectionUnknown means the direction could not be determined.
	TapDirectionUnknown TapDirection = "UNKNOWN"
)

// TapInValidationResult is the only direction literal the Android client
// compares against when a scan succeeds. Any other non-empty value is treated
// as a tap-out; no tap-out constant exists in the v1.6.22 dex.
const TapInValidationResult = "CHECKIN"

// QR validation error codes reported by the backend on a failed scan.
const (
	QRValidationInvalidQR      = "INVALID_QR"
	QRValidationTicketNotFound = "TICKET_NOT_FOUND"
	QRValidationLegsNotFound   = "LEGS_NOT_FOUND"
	QRValidationScanFailed     = "SCAN_FAILED"
	QRValidationAlreadyUsed    = "ALREADY_USED"
	QRValidationScanInFirst    = "SCAN_IN_FIRST"
	QRValidationFailed         = "FAILED"
)

// GateQRValidation is the outcome of a validate call.
type GateQRValidation struct {
	// Result is the raw data string returned by the backend (e.g. "CHECKIN").
	Result string `json:"result"`
	// Direction is the tap direction derived from Result.
	Direction TapDirection `json:"direction"`
}

// DirectionFromValidation maps a validate response string to a tap direction.
// It mirrors the Android client, which treats "CHECKIN" (case-insensitive) as a
// tap-in and every other non-empty value as a tap-out.
func DirectionFromValidation(result string) TapDirection {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return TapDirectionUnknown
	}
	if strings.EqualFold(trimmed, TapInValidationResult) {
		return TapDirectionIn
	}
	return TapDirectionOut
}
