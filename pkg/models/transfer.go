package models

// Ticket transfer models for the JakLingko peer-to-peer ticket transfer flow.
//
// Static analysis reference (JakLingko Android v1.6.22):
//
//	GET  auth/v3/profile/by-phone/{phone}  -> {user_id, email, name, phone, lang}
//	POST ticket/v3/ticket/transfer         {pin, destination_uid, ticket_id, phone, firstname, lastname, language}
//
// The transfer request body is encrypted by the app's OkHttp interceptor and
// the endpoint answers with a status envelope only (app model api.c), so success
// is determined by message == "SUCCESS".

// TransferSuccessMessage is the message the backend returns on a successful
// ticket transfer. The Android client compares api.c.message against this
// literal before showing the success screen.
const TransferSuccessMessage = "SUCCESS"

// UserByPhone is the recipient profile returned by
// GET auth/v3/profile/by-phone/{phone} (app model xe.b).
type UserByPhone struct {
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Lang   string `json:"lang"`
}

// TransferTicketRequest is the encrypted body for POST ticket/v3/ticket/transfer
// (app model xe.a).
type TransferTicketRequest struct {
	PIN            string `json:"pin"`
	DestinationUID int    `json:"destination_uid"`
	TicketID       string `json:"ticket_id"`
	Phone          string `json:"phone"`
	Firstname      string `json:"firstname"`
	Lastname       string `json:"lastname"`
	Language       string `json:"language"`
}

// StatusResult mirrors the app's api.c envelope, which carries no data payload.
type StatusResult struct {
	StatusCode   int      `json:"statusCode"`
	Message      string   `json:"message"`
	TitleMessage string   `json:"title_message,omitempty"`
	Error        []string `json:"error,omitempty"`
}

// TransferResult is the outcome of a ticket transfer, including the resolved
// recipient profile so callers can display or audit it.
type TransferResult struct {
	TicketID  string       `json:"ticket_id"`
	Recipient UserByPhone  `json:"recipient"`
	Status    StatusResult `json:"status"`
}
