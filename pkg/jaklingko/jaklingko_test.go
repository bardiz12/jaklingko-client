package jaklingko_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bardiz12/jaklingko-client/pkg/jaklingko"
	"github.com/bardiz12/jaklingko-client/pkg/models"
)

func TestClient_MultiAccountIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	sessPathA := filepath.Join(tmpDir, "session_a.json")
	sessPathB := filepath.Join(tmpDir, "session_b.json")

	// Instantiate Account A
	clientA := jaklingko.New(
		jaklingko.WithSessionFile(sessPathA),
		jaklingko.WithCredentials("62859106800001", "passA"),
	)

	// Instantiate Account B
	clientB := jaklingko.New(
		jaklingko.WithSessionFile(sessPathB),
		jaklingko.WithCredentials("62859106800002", "passB"),
	)

	if clientA.HasValidSession() {
		t.Errorf("expected clientA to have no session initially")
	}
	if clientB.HasValidSession() {
		t.Errorf("expected clientB to have no session initially")
	}

	// Set session on Account A
	sessA := &models.Session{
		AccessToken: "token_account_a",
		Username:    "62859106800001",
		SavedAt:     time.Now(),
	}
	if err := clientA.SetSession(sessA); err != nil {
		t.Fatalf("failed to set session A: %v", err)
	}

	// Verify Account A has session, but Account B still does not
	if !clientA.HasValidSession() {
		t.Errorf("expected clientA to have valid session")
	}
	if clientB.HasValidSession() {
		t.Errorf("expected clientB to remain unauthenticated, but it has session")
	}

	// Set session on Account B
	sessB := &models.Session{
		AccessToken: "token_account_b",
		Username:    "62859106800002",
		SavedAt:     time.Now(),
	}
	if err := clientB.SetSession(sessB); err != nil {
		t.Fatalf("failed to set session B: %v", err)
	}

	// Verify both accounts maintain their own distinct sessions
	gotA := clientA.GetSession()
	gotB := clientB.GetSession()

	if gotA.AccessToken != "token_account_a" || gotA.Username != "62859106800001" {
		t.Errorf("unexpected session A data: %+v", gotA)
	}
	if gotB.AccessToken != "token_account_b" || gotB.Username != "62859106800002" {
		t.Errorf("unexpected session B data: %+v", gotB)
	}

	// Clear Account A session and verify Account B is unaffected
	if err := clientA.Logout(nil); err != nil {
		t.Fatalf("failed to logout client A: %v", err)
	}

	if clientA.HasValidSession() {
		t.Errorf("expected clientA to be logged out")
	}
	if !clientB.HasValidSession() {
		t.Errorf("expected clientB to retain active session after clientA logged out")
	}

	// Check on disk files
	if _, err := os.Stat(sessPathA); !os.IsNotExist(err) {
		t.Errorf("expected sessPathA to be deleted on disk")
	}
	if _, err := os.Stat(sessPathB); err != nil {
		t.Errorf("expected sessPathB to still exist on disk")
	}
}

func TestNormalizePhoneNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"081234567890", "6281234567890"},
		{"+6281234567890", "6281234567890"},
		{"6281234567890", "6281234567890"},
		{"0812-3456-7890", "6281234567890"},
		{" 0812 3456 7890 ", "6281234567890"},
	}

	for _, tt := range tests {
		got := jaklingko.NormalizePhoneNumber(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizePhoneNumber(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildBookingPayload(t *testing.T) {
	fare := models.RouteFare{
		ID:          "itinerary-12345",
		TotalAmount: 3500,
		PTOCost:     3500,
		Origin: models.StopLocation{
			Name: "Stasiun Dukuh Atas",
		},
		Destination: models.StopLocation{
			Name: "Halte Monas",
		},
	}

	payload := jaklingko.BuildBookingPayload(fare, jaklingko.PaymentChannelQRIS)
	if payload.ItinerariesID != "itinerary-12345" {
		t.Errorf("unexpected itinerariesId: %s", payload.ItinerariesID)
	}
	if payload.PaymentChannel != "4" {
		t.Errorf("unexpected payment channel: %s", payload.PaymentChannel)
	}
	if payload.Amount != "3500" {
		t.Errorf("unexpected amount: %s", payload.Amount)
	}
	if payload.Currency != "360" {
		t.Errorf("unexpected currency: %s", payload.Currency)
	}
	if payload.OriginLabel != "Stasiun Dukuh Atas" {
		t.Errorf("unexpected origin label: %s", payload.OriginLabel)
	}
	if payload.DestinationLabel != "Halte Monas" {
		t.Errorf("unexpected destination label: %s", payload.DestinationLabel)
	}
}

func TestClient_ConfigOverrides(t *testing.T) {
	// Test environment variable overrides
	os.Setenv("JAKLINGKO_BASE_URL", "https://custom-gateway.example.com")
	os.Setenv("JAKLINGKO_CIPHER_SALT", "custom-salt-123")
	os.Setenv("JAKLINGKO_DETAIL_SALT", "custom-detail-456")
	os.Setenv("JAKLINGKO_USER_AGENT", "okhttp/custom-env-ua")
	defer func() {
		os.Unsetenv("JAKLINGKO_BASE_URL")
		os.Unsetenv("JAKLINGKO_CIPHER_SALT")
		os.Unsetenv("JAKLINGKO_DETAIL_SALT")
		os.Unsetenv("JAKLINGKO_USER_AGENT")
	}()

	client := jaklingko.New(
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)
	if !client.HasValidSession() && client.GetSession() != nil {
		t.Errorf("expected clean session")
	}

	// Test explicit functional options override env vars
	explicitClient := jaklingko.New(
		jaklingko.WithBaseURL("https://explicit-gateway.example.com"),
		jaklingko.WithCipherSalt("explicit-cipher-salt"),
		jaklingko.WithDetailSalt("explicit-detail-salt"),
		jaklingko.WithUserAgent("custom-explicit-ua"),
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)
	if explicitClient == nil {
		t.Fatalf("expected client to be initialized")
	}
}

func TestClient_HeadersAndUserAgent(t *testing.T) {
	var capturedUA string
	var capturedCacheControl string
	var capturedPragma string
	var capturedExpires string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		capturedCacheControl = r.Header.Get("Cache-Control")
		capturedPragma = r.Header.Get("Pragma")
		capturedExpires = r.Header.Get("Expires")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Default client should have okhttp/4.12.0 and NO browser cache headers
	client := jaklingko.New(
		jaklingko.WithBaseURL(server.URL),
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)

	_, _, err := client.DoRequest(context.Background(), "GET", "test-endpoint", nil, false)
	if err != nil {
		t.Fatalf("DoRequest failed: %v", err)
	}

	if capturedUA != jaklingko.DefaultUserAgent {
		t.Errorf("expected User-Agent %q, got %q", jaklingko.DefaultUserAgent, capturedUA)
	}
	if capturedCacheControl != "" {
		t.Errorf("expected no Cache-Control header, got %q", capturedCacheControl)
	}
	if capturedPragma != "" {
		t.Errorf("expected no Pragma header, got %q", capturedPragma)
	}
	if capturedExpires != "" {
		t.Errorf("expected no Expires header, got %q", capturedExpires)
	}

	// Custom User-Agent override
	clientCustomUA := jaklingko.New(
		jaklingko.WithBaseURL(server.URL),
		jaklingko.WithUserAgent("okhttp/5.0.0-alpha.12"),
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)
	_, _, err = clientCustomUA.DoRequest(context.Background(), "GET", "test-endpoint", nil, false)
	if err != nil {
		t.Fatalf("DoRequest failed: %v", err)
	}
	if capturedUA != "okhttp/5.0.0-alpha.12" {
		t.Errorf("expected overridden User-Agent %q, got %q", "okhttp/5.0.0-alpha.12", capturedUA)
	}
}

func TestClient_BalanceQueries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/payment/v3/channels/list":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"statusCode": 200,
				"message": "Success",
				"data": [
					{"id": "4", "name": "QRIS", "category": "qr", "enable": true, "statusBinding": true},
					{"id": "5", "name": "Fello", "category": "ewallet", "enable": true, "statusBinding": true},
					{"id": "14", "name": "OVO", "category": "ewallet", "enable": true, "statusBinding": false}
				]
			}`))
		case "/payment/v3/channels/balance/5":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"statusCode": 200,
				"message": "Success",
				"data": {
					"balance": "Rp. 7.000,00",
					"balance_unformated": "7000.00"
				}
			}`))
		case "/payment/v3/channels/balance/14":
			w.WriteHeader(http.StatusNotAcceptable)
			_, _ = w.Write([]byte(`{
				"statusCode": 406,
				"message": "NO_BINDING_FOUND",
				"error": ["NO_BINDING_FOUND"]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := jaklingko.New(
		jaklingko.WithBaseURL(server.URL),
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)

	// Test GetChannelBalance for Fello (5)
	bal, err := client.GetChannelBalance(context.Background(), "5")
	if err != nil {
		t.Fatalf("expected GetChannelBalance to succeed, got: %v", err)
	}
	if bal.Balance != "Rp. 7.000,00" {
		t.Errorf("unexpected balance: %s", bal.Balance)
	}
	if bal.BalanceUnformatted != "7000.00" {
		t.Errorf("unexpected unformatted balance: %s", bal.BalanceUnformatted)
	}

	// Test GetChannelBalance for unbound OVO (14)
	_, err = client.GetChannelBalance(context.Background(), "14")
	if err == nil {
		t.Errorf("expected error for unbound channel 14")
	}

	// Test GetAllBalances
	balances, err := client.GetAllBalances(context.Background())
	if err != nil {
		t.Fatalf("expected GetAllBalances to succeed, got: %v", err)
	}
	if len(balances) != 2 {
		t.Fatalf("expected 2 e-wallet channels, got %d", len(balances))
	}

	// Fello check
	if balances[0].ChannelID != "5" || !balances[0].Bound || balances[0].Balance != "Rp. 7.000,00" {
		t.Errorf("unexpected Fello balance info: %+v", balances[0])
	}
	// OVO check
	if balances[1].ChannelID != "14" || balances[1].Bound || balances[1].Error != "NO_BINDING_FOUND" {
		t.Errorf("unexpected OVO balance info: %+v", balances[1])
	}
}
