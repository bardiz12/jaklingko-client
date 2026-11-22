package jaklingko_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bardiz12/jaklingko-client/pkg/jaklingko"
	"github.com/bardiz12/jaklingko-client/pkg/models"
)

func newTransferTestClient(t *testing.T, baseURL string) jaklingko.Client {
	t.Helper()
	return jaklingko.New(
		jaklingko.WithBaseURL(baseURL),
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
		jaklingko.WithCipherSalt("test-cipher-salt"),
		jaklingko.WithDetailSalt("test-detail-salt"),
	)
}

func TestSplitRecipientName(t *testing.T) {
	cases := []struct {
		name      string
		wantFirst string
		wantLast  string
	}{
		{"Budi Santoso", "Budi", "Santoso"},
		{"Budi", "Budi", ""},
		{"  Budi   Santoso  ", "Budi", "Santoso"},
		{"Budi Santoso Wijaya", "Budi", "Wijaya"},
		{"", "", ""},
		{"   ", "", ""},
	}

	for _, tc := range cases {
		first, last := jaklingko.SplitRecipientName(tc.name)
		if first != tc.wantFirst || last != tc.wantLast {
			t.Errorf("SplitRecipientName(%q) = (%q, %q), want (%q, %q)", tc.name, first, last, tc.wantFirst, tc.wantLast)
		}
	}
}

func TestClient_GetProfileByPhone(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"statusCode": 200,
			"message": "SUCCESS",
			"data": {
				"user_id": 42,
				"email": "recipient@example.com",
				"name": "Budi Santoso",
				"phone": "6281234567890",
				"lang": "id"
			}
		}`))
	}))
	defer server.Close()

	client := newTransferTestClient(t, server.URL)
	profile, err := client.GetProfileByPhone(context.Background(), "0812-3456-7890")
	if err != nil {
		t.Fatalf("GetProfileByPhone failed: %v", err)
	}

	if gotPath != "/auth/v3/profile/by-phone/6281234567890" {
		t.Errorf("unexpected lookup path: %q", gotPath)
	}
	if profile.UserID != 42 || profile.Name != "Budi Santoso" || profile.Phone != "6281234567890" || profile.Lang != "id" {
		t.Errorf("unexpected profile: %+v", profile)
	}
}

func TestClient_GetProfileByPhone_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode": 404, "message": "NOT_FOUND", "error": ["USER_NOT_FOUND"]}`))
	}))
	defer server.Close()

	client := newTransferTestClient(t, server.URL)
	_, err := client.GetProfileByPhone(context.Background(), "081234567890")
	if err == nil {
		t.Fatal("expected error for unknown phone number")
	}
	if !strings.Contains(err.Error(), "USER_NOT_FOUND") {
		t.Errorf("expected USER_NOT_FOUND in error, got: %v", err)
	}
}

func TestClient_TransferTicket_Success(t *testing.T) {
	var gotPath, gotMethod, gotEncrypted string
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotEncrypted = r.Header.Get("X-Encrypted")
		gotBody, _ = io.ReadAll(r.Body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"statusCode": 200, "message": "SUCCESS"}`))
	}))
	defer server.Close()

	client := newTransferTestClient(t, server.URL)
	status, err := client.TransferTicket(context.Background(), models.TransferTicketRequest{
		PIN:            "123456",
		DestinationUID: 42,
		TicketID:       "T-TEST123",
		Phone:          "6281234567890",
		Firstname:      "Budi",
		Lastname:       "Santoso",
		Language:       "id",
	})
	if err != nil {
		t.Fatalf("TransferTicket failed: %v", err)
	}

	if gotPath != "/ticket/v3/ticket/transfer" || gotMethod != http.MethodPost {
		t.Errorf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotEncrypted != "true" {
		t.Errorf("expected X-Encrypted: true, got %q", gotEncrypted)
	}

	var envelope map[string]string
	if err := json.Unmarshal(gotBody, &envelope); err != nil {
		t.Fatalf("request body is not a JSON envelope: %v (raw: %s)", err, string(gotBody))
	}
	if envelope["request"] == "" {
		t.Errorf("expected encrypted 'request' field, got: %s", string(gotBody))
	}
	if strings.Contains(string(gotBody), "destination_uid") || strings.Contains(string(gotBody), "123456") {
		t.Errorf("request body leaked plaintext fields: %s", string(gotBody))
	}
	if status.Message != models.TransferSuccessMessage {
		t.Errorf("unexpected status: %+v", status)
	}
}

func TestClient_TransferTicket_Rejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"statusCode": 400, "message": "FAILED", "error": ["TICKET_ALREADY_USED"]}`))
	}))
	defer server.Close()

	client := newTransferTestClient(t, server.URL)
	_, err := client.TransferTicket(context.Background(), models.TransferTicketRequest{
		PIN:            "123456",
		DestinationUID: 42,
		TicketID:       "T-TEST123",
		Phone:          "6281234567890",
	})
	if err == nil {
		t.Fatal("expected error for non-SUCCESS response")
	}
	if !strings.Contains(err.Error(), "TICKET_ALREADY_USED") {
		t.Errorf("expected TICKET_ALREADY_USED in error, got: %v", err)
	}
}

func TestClient_TransferTicket_ValidationErrors(t *testing.T) {
	client := jaklingko.New(jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")))

	cases := []models.TransferTicketRequest{
		{DestinationUID: 42, TicketID: "T-1", Phone: "6281234567890"}, // missing PIN
		{PIN: "123456", TicketID: "T-1", Phone: "6281234567890"},      // missing destination UID
		{PIN: "123456", DestinationUID: 42, Phone: "6281234567890"},   // missing ticket ID
		{PIN: "123456", DestinationUID: 42, TicketID: "T-1"},          // missing phone
	}
	for i, req := range cases {
		if _, err := client.TransferTicket(context.Background(), req); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func TestClient_TransferTicketToPhone(t *testing.T) {
	var transferCalled atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/auth/v3/profile/by-phone/6281234567890":
			_, _ = w.Write([]byte(`{
				"statusCode": 200, "message": "SUCCESS",
				"data": {"user_id": 42, "email": "r@example.com", "name": "Budi Santoso", "phone": "6281234567890", "lang": "id"}
			}`))
		case strings.HasPrefix(r.URL.Path, "/ticket/v3/ticket/detail/"):
			_, _ = w.Write([]byte(`{
				"statusCode": 200, "message": "SUCCESS",
				"data": {"ticket": {"ticket_longid": "T-TEST123", "in_journey": false, "is_used": false}}
			}`))
		case r.URL.Path == "/ticket/v3/ticket/transfer":
			transferCalled.Store(true)
			_, _ = w.Write([]byte(`{"statusCode": 200, "message": "SUCCESS"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTransferTestClient(t, server.URL)
	if err := client.SetSession(&models.Session{
		AccessToken: "sender-token",
		Username:    "6289999999999",
		SavedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("failed to set session: %v", err)
	}

	result, err := client.TransferTicketToPhone(context.Background(), "T-TEST123", "0812-3456-7890", "123456")
	if err != nil {
		t.Fatalf("TransferTicketToPhone failed: %v", err)
	}
	if !transferCalled.Load() {
		t.Error("expected the transfer endpoint to be called")
	}
	if result.TicketID != "T-TEST123" || result.Recipient.UserID != 42 || result.Status.Message != models.TransferSuccessMessage {
		t.Errorf("unexpected transfer result: %+v", result)
	}
}

func TestClient_TransferTicketToPhone_RejectsSelfTransfer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"statusCode": 200, "message": "SUCCESS",
			"data": {"user_id": 42, "email": "r@example.com", "name": "Budi", "phone": "6281234567890", "lang": "id"}
		}`))
	}))
	defer server.Close()

	client := newTransferTestClient(t, server.URL)
	if err := client.SetSession(&models.Session{
		AccessToken: "sender-token",
		Username:    "081234567890",
		SavedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("failed to set session: %v", err)
	}

	_, err := client.TransferTicketToPhone(context.Background(), "T-TEST123", "081234567890", "123456")
	if err == nil {
		t.Fatal("expected self-transfer to be rejected")
	}
	if !strings.Contains(err.Error(), "own account") {
		t.Errorf("expected own-account error, got: %v", err)
	}
}

func TestClient_TransferTicketToPhone_RejectsTicketInUse(t *testing.T) {
	var transferCalled atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/auth/v3/profile/by-phone/6281234567890":
			_, _ = w.Write([]byte(`{
				"statusCode": 200, "message": "SUCCESS",
				"data": {"user_id": 42, "email": "r@example.com", "name": "Budi", "phone": "6281234567890", "lang": "id"}
			}`))
		case strings.HasPrefix(r.URL.Path, "/ticket/v3/ticket/detail/"):
			_, _ = w.Write([]byte(`{
				"statusCode": 200, "message": "SUCCESS",
				"data": {"ticket": {"ticket_longid": "T-TEST123", "in_journey": true, "is_used": false}}
			}`))
		case r.URL.Path == "/ticket/v3/ticket/transfer":
			transferCalled.Store(true)
			_, _ = w.Write([]byte(`{"statusCode": 200, "message": "SUCCESS"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTransferTestClient(t, server.URL)
	_, err := client.TransferTicketToPhone(context.Background(), "T-TEST123", "081234567890", "123456")
	if err == nil {
		t.Fatal("expected in-use ticket to be rejected")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("expected in-use error, got: %v", err)
	}
	if transferCalled.Load() {
		t.Error("transfer endpoint must not be called for an in-use ticket")
	}
}
