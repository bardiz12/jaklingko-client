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

func TestClient_SearchLocation_ModesArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/route/v4/route/position" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"statusCode": 200,
			"message": "SUCCESS",
			"data": [
				{
					"label": "Stasiun MRT Bundaran HI",
					"modes": ["TRANSIT_TRAIN"],
					"latitude": -6.193,
					"longitude": 106.822
				}
			]
		}`))
	}))
	defer server.Close()

	client := jaklingko.New(
		jaklingko.WithBaseURL(server.URL),
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)

	stops, err := client.SearchLocation(context.Background(), "Bundaran HI", 5)
	if err != nil {
		t.Fatalf("SearchLocation failed: %v", err)
	}
	if len(stops) != 1 {
		t.Fatalf("expected 1 stop, got %d", len(stops))
	}
	if len(stops[0].Modes) != 1 || stops[0].Modes[0] != "TRANSIT_TRAIN" {
		t.Errorf("unexpected modes: %v", stops[0].Modes)
	}
	if stops[0].GetPTOName() != "MRTJ" {
		t.Errorf("unexpected PTO: %q", stops[0].GetPTOName())
	}
}

func TestClient_SearchTicketFare_ModesArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/route/v4/1/route" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"statusCode": 200,
			"message": "SUCCESS",
			"data": [
				{
					"id": "route-1",
					"ptoCost": 3500,
					"totalAmount": 3500,
					"origin": {"label": "Stasiun MRT Bundaran HI", "modes": ["TRANSIT_TRAIN"]},
					"destination": {"label": "Halte TransJakarta Monas", "modes": ["TRANSIT_BUS", "TRANSIT_TRAIN"]},
					"legs": [
						{
							"id": "leg-1",
							"origin": {"label": "Stasiun MRT Bundaran HI", "modes": ["TRANSIT_TRAIN"]},
							"destination": {"label": "Halte TransJakarta Monas", "modes": ["TRANSIT_BUS"]}
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	client := jaklingko.New(
		jaklingko.WithBaseURL(server.URL),
		jaklingko.WithSessionFile(filepath.Join(t.TempDir(), ".session.json")),
	)

	origin := models.StopLocation{Label: "Bundaran HI", Latitude: -6.193, Longitude: 106.822}
	destination := models.StopLocation{Label: "Monas", Latitude: -6.175, Longitude: 106.827}

	fares, err := client.SearchTicketFare(context.Background(), origin, destination, nil)
	if err != nil {
		t.Fatalf("SearchTicketFare failed: %v", err)
	}
	if len(fares) != 1 {
		t.Fatalf("expected 1 fare option, got %d", len(fares))
	}
	if len(fares[0].Destination.Modes) != 2 {
		t.Errorf("unexpected destination modes: %v", fares[0].Destination.Modes)
	}
}
