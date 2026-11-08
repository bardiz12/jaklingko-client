package jaklingko_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/bardiz12/jaklingko-client/pkg/jaklingko"
	"github.com/bardiz12/jaklingko-client/pkg/models"
)

func TestPayFelloTicket_ValidationErrors(t *testing.T) {
	client := jaklingko.New(
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)

	// Empty URL
	if err := client.PayFelloTicket(context.Background(), "", "123456"); err == nil {
		t.Errorf("expected error for empty URL")
	}

	// Short PIN
	if err := client.PayFelloTicket(context.Background(), "https://example.com?tID=abc", "123"); err == nil {
		t.Errorf("expected error for non-6-digit PIN")
	}

	// Missing tID param
	if err := client.PayFelloTicket(context.Background(), "https://example.com/nowhere", "123456"); err == nil {
		t.Errorf("expected error for missing tID parameter")
	}
}

func TestPayFelloTicket_MockFlow(t *testing.T) {
	// Mock JakLingko callback receiver
	cbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("status") != "PROCESSED" {
			http.Error(w, "bad status", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"statusCode":200,"message":"PAYMENT_SUCCESS"}`))
	}))
	defer cbServer.Close()

	// Mock Fello gateway server
	felloServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Api/Wallet/V1/Payment/Purchase/Request":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><form action="/Api/Wallet/V1/Payment/Purchase/Verify?tID=mock_tid" method="POST"></form></html>`))
		case "/Api/Wallet/V1/Payment/Purchase/Verify":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><input name="fromMember" value="081234567890" /><input name="ticketID" value="mock_tid" /></html>`))
		case "/Api/Wallet/V1/Payment/Validation/credentialValidation":
			_ = r.ParseForm()
			if r.FormValue("credential") == "123456" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"responseMessage":"VALID"}`))
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"responseMessage":"WRONG"}`))
			}
		case "/Api/Wallet/V1/Payment/Purchase/ConfirmPin":
			w.WriteHeader(http.StatusOK)
			respHTML := `<html>
				<form id="form-redirect" action="` + cbServer.URL + `" method="POST">
					<input type="hidden" name="amount" value="3000.00" />
					<input type="hidden" name="status" value="PROCESSED" />
				</form>
			</html>`
			_, _ = w.Write([]byte(respHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	defer felloServer.Close()

	client := jaklingko.New(
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)
	_ = client.SetSession(&models.Session{
		Username:    "081234567890",
		AccessToken: "mock_token",
	})

	// Valid flow
	targetURL := felloServer.URL + "/Api/Wallet/V1/Payment/Purchase/Request?tID=mock_tid"
	err := client.PayFelloTicket(context.Background(), targetURL, "123456")
	if err != nil {
		t.Fatalf("expected successful fello payment, got: %v", err)
	}

	// Invalid PIN
	err = client.PayFelloTicket(context.Background(), targetURL, "999999")
	if err == nil {
		t.Fatalf("expected error for wrong PIN")
	}
}
