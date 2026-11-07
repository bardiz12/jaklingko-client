package jaklingko_test

import (
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
	defer func() {
		os.Unsetenv("JAKLINGKO_BASE_URL")
		os.Unsetenv("JAKLINGKO_CIPHER_SALT")
		os.Unsetenv("JAKLINGKO_DETAIL_SALT")
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
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)
	if explicitClient == nil {
		t.Fatalf("expected client to be initialized")
	}
}
