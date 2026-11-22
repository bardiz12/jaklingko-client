package jaklingko_test

import (
	"context"
	"errors"
	"fmt"
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

func TestDirectionFromValidation(t *testing.T) {
	tests := []struct {
		input string
		want  models.TapDirection
	}{
		{"CHECKIN", models.TapDirectionIn},
		{"checkin", models.TapDirectionIn},
		{"  CHECKIN  ", models.TapDirectionIn},
		{"CHECKOUT", models.TapDirectionOut},
		{"", models.TapDirectionUnknown},
		{"   ", models.TapDirectionUnknown},
	}

	for _, tt := range tests {
		if got := models.DirectionFromValidation(tt.input); got != tt.want {
			t.Errorf("DirectionFromValidation(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

// ticketDetailJSON builds a ticket/v3/ticket/detail response with a walking leg,
// a bus leg and a train leg.
func ticketDetailJSON(inJourney bool, busActive bool) string {
	return fmt.Sprintf(`{
		"statusCode": 200,
		"message": "Success",
		"data": {
			"ticket": {
				"id": 4242,
				"ticket_longid": "T-TEST",
				"in_journey": %t,
				"is_used": false
			},
			"itineraries": {
				"id": "it-1",
				"legs": [
					{"id": 11, "mode": "FOOT", "is_active": false, "is_passed": false,
					 "origin": {"name": "Origin Walk"}, "destination": {"name": "Halte A"}},
					{"id": 22, "mode": "TRANSIT_BUS", "is_active": %t, "is_passed": false,
					 "origin": {"name": "Halte A"}, "destination": {"name": "Stasiun C"}},
					{"id": 33, "mode": "TRANSIT_TRAIN", "is_active": false, "is_passed": false,
					 "origin": {"name": "Stasiun C"}, "destination": {"name": "Stasiun D"}}
				]
			}
		}
	}`, inJourney, busActive)
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (jaklingko.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := jaklingko.New(
		jaklingko.WithBaseURL(server.URL),
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)
	return client, server
}

func TestClient_GateQRActivation(t *testing.T) {
	var generateCalls, refreshCalls int32

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/ticket/v3/ticket/detail/"):
			_, _ = w.Write([]byte(ticketDetailJSON(false, true)))
		case r.URL.Path == "/ticket/v3/qr-activation/generate":
			n := atomic.AddInt32(&generateCalls, 1)
			_, _ = fmt.Fprintf(w, `{"statusCode":200,"message":"Success","data":{"qr_string":"JLI#gen-%d"}}`, n)
		case r.URL.Path == "/ticket/v3/qr-activation/refresh":
			atomic.AddInt32(&refreshCalls, 1)
			_, _ = w.Write([]byte(`{"statusCode":200,"message":"Success","data":{"qr_string":"JLI#refresh"}}`))
		case r.URL.Path == "/ticket/v3/qr-activation/validate":
			_, _ = w.Write([]byte(`{"statusCode":200,"message":"Success","data":"CHECKIN"}`))
		default:
			http.NotFound(w, r)
		}
	})

	ctx := context.Background()

	qr, err := client.GenerateGateQR(ctx, 4242, 22)
	if err != nil {
		t.Fatalf("GenerateGateQR failed: %v", err)
	}
	if qr.QRString != "JLI#gen-1" {
		t.Errorf("unexpected generated QR: %q", qr.QRString)
	}

	refreshed, err := client.RefreshGateQR(ctx, 4242)
	if err != nil {
		t.Fatalf("RefreshGateQR failed: %v", err)
	}
	if refreshed.QRString != "JLI#refresh" {
		t.Errorf("unexpected refreshed QR: %q", refreshed.QRString)
	}
	if atomic.LoadInt32(&refreshCalls) != 1 {
		t.Errorf("expected 1 refresh call, got %d", refreshCalls)
	}

	res, err := client.ValidateGateQR(ctx, 4242, 22, "JLI#gen-1")
	if err != nil {
		t.Fatalf("ValidateGateQR failed: %v", err)
	}
	if res.Result != "CHECKIN" || res.Direction != models.TapDirectionIn {
		t.Errorf("unexpected validation result: %+v", res)
	}
}

func TestClient_ValidateGateQRError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ticket/v3/qr-activation/validate" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = w.Write([]byte(`{"statusCode":406,"message":"INVALID_QR","error":["INVALID_QR"]}`))
	})

	_, err := client.ValidateGateQR(context.Background(), 4242, 22, "JLI#bad")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), models.QRValidationInvalidQR) {
		t.Errorf("expected error to mention %s, got: %v", models.QRValidationInvalidQR, err)
	}
}

func TestClient_WatchGateQR(t *testing.T) {
	var generateCalls int32

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ticket/v3/qr-activation/generate" {
			http.NotFound(w, r)
			return
		}
		n := atomic.AddInt32(&generateCalls, 1)
		_, _ = fmt.Fprintf(w, `{"statusCode":200,"message":"Success","data":{"qr_string":"JLI#cycle-%d"}}`, n)
	})

	var frames []jaklingko.QRFrame
	err := client.WatchGateQR(context.Background(), 4242, 22, &jaklingko.WatchOptions{
		Interval:  5 * time.Millisecond,
		MaxCycles: 3,
		OnFrame: func(f jaklingko.QRFrame) {
			frames = append(frames, f)
		},
	})
	if err != nil {
		t.Fatalf("WatchGateQR failed: %v", err)
	}

	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	for i, f := range frames {
		if f.Cycle != i+1 {
			t.Errorf("frame %d: expected cycle %d, got %d", i, i+1, f.Cycle)
		}
		if f.Err != nil {
			t.Errorf("frame %d: unexpected error: %v", i, f.Err)
		}
		want := fmt.Sprintf("JLI#cycle-%d", i+1)
		if f.QR == nil || f.QR.QRString != want {
			t.Errorf("frame %d: expected QR %q, got %+v", i, want, f.QR)
		}
	}
}

func TestClient_WatchGateQR_ContextCancel(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"statusCode":200,"message":"Success","data":{"qr_string":"JLI#cycle"}}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.WatchGateQR(ctx, 4242, 22, &jaklingko.WatchOptions{
		Interval: time.Hour,
		OnFrame: func(f jaklingko.QRFrame) {
			cancel()
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestClient_WatchGateQR_StopsOnError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"statusCode":500,"message":"FAILED","error":["FAILED"]}`))
	})

	var frames []jaklingko.QRFrame
	err := client.WatchGateQR(context.Background(), 4242, 22, &jaklingko.WatchOptions{
		Interval:  time.Millisecond,
		MaxCycles: 5,
		OnFrame: func(f jaklingko.QRFrame) {
			frames = append(frames, f)
		},
	})
	if err == nil {
		t.Fatal("expected watch to stop with an error")
	}
	if len(frames) != 1 {
		t.Fatalf("expected a single failed frame, got %d", len(frames))
	}
	if frames[0].Err == nil {
		t.Error("expected frame to carry the generation error")
	}
}

func TestClient_ResolveTapTarget(t *testing.T) {
	t.Run("tap-in picks first unpassed transit leg", func(t *testing.T) {
		client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(ticketDetailJSON(false, true)))
		})

		target, err := client.ResolveTapTarget(context.Background(), "T-TEST", 0)
		if err != nil {
			t.Fatalf("ResolveTapTarget failed: %v", err)
		}
		if target.TicketID != 4242 {
			t.Errorf("expected ticket ID 4242, got %d", target.TicketID)
		}
		if target.LegsID != 22 {
			t.Errorf("expected leg 22, got %d", target.LegsID)
		}
		if target.Direction != models.TapDirectionIn {
			t.Errorf("expected TAP_IN, got %s", target.Direction)
		}
		if target.Origin != "Halte A" || target.Destination != "Stasiun C" {
			t.Errorf("unexpected leg labels: %q -> %q", target.Origin, target.Destination)
		}
	})

	t.Run("tap-out targets the leg in journey", func(t *testing.T) {
		client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(ticketDetailJSON(true, true)))
		})

		target, err := client.ResolveTapTarget(context.Background(), "T-TEST", 0)
		if err != nil {
			t.Fatalf("ResolveTapTarget failed: %v", err)
		}
		if target.Direction != models.TapDirectionOut {
			t.Errorf("expected TAP_OUT, got %s", target.Direction)
		}
		if target.LegsID != 22 {
			t.Errorf("expected active leg 22, got %d", target.LegsID)
		}
	})

	t.Run("explicit leg overrides selection", func(t *testing.T) {
		client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(ticketDetailJSON(false, true)))
		})

		target, err := client.ResolveTapTarget(context.Background(), "T-TEST", 33)
		if err != nil {
			t.Fatalf("ResolveTapTarget failed: %v", err)
		}
		if target.LegsID != 33 {
			t.Errorf("expected explicit leg 33, got %d", target.LegsID)
		}
	})

	t.Run("unknown leg returns error", func(t *testing.T) {
		client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(ticketDetailJSON(false, true)))
		})

		if _, err := client.ResolveTapTarget(context.Background(), "T-TEST", 99); err == nil {
			t.Fatal("expected error for unknown leg ID")
		}
	})
}
