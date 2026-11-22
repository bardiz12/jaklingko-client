package models_test

import (
	"encoding/json"
	"testing"

	"github.com/bardiz12/jaklingko-client/pkg/models"
)

func TestStopLocation_UnmarshalModes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want models.StopModes
	}{
		{"string", `{"label":"Halte Bus","modes":"TRANSIT_BUS"}`, models.StopModes{"TRANSIT_BUS"}},
		{"array", `{"label":"Stasiun","modes":["TRANSIT_TRAIN","TRANSIT_BUS"]}`, models.StopModes{"TRANSIT_TRAIN", "TRANSIT_BUS"}},
		{"single element array", `{"label":"Stasiun","modes":["TRANSIT_TRAIN"]}`, models.StopModes{"TRANSIT_TRAIN"}},
		{"empty string", `{"label":"X","modes":""}`, nil},
		{"null", `{"label":"X","modes":null}`, nil},
		{"missing", `{"label":"X"}`, nil},
		{"empty array", `{"label":"X","modes":[]}`, models.StopModes{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stop models.StopLocation
			if err := json.Unmarshal([]byte(tc.raw), &stop); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if len(stop.Modes) != len(tc.want) {
				t.Fatalf("got modes %v, want %v", stop.Modes, tc.want)
			}
			for i := range tc.want {
				if stop.Modes[i] != tc.want[i] {
					t.Errorf("mode[%d] = %q, want %q", i, stop.Modes[i], tc.want[i])
				}
			}
		})
	}
}

func TestStopLocation_UnmarshalModes_InvalidType(t *testing.T) {
	var stop models.StopLocation
	if err := json.Unmarshal([]byte(`{"modes":123}`), &stop); err == nil {
		t.Fatal("expected error for numeric modes")
	}
}

func TestStopModes_MarshalJSON(t *testing.T) {
	cases := []struct {
		name  string
		modes models.StopModes
		want  string
	}{
		{"nil", nil, `""`},
		{"single", models.StopModes{"TRANSIT_BUS"}, `"TRANSIT_BUS"`},
		{"multiple", models.StopModes{"TRANSIT_BUS", "TRANSIT_TRAIN"}, `["TRANSIT_BUS","TRANSIT_TRAIN"]`},
		{"empty", models.StopModes{}, `[]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.modes)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestStopLocation_GetPTOName_WithModesArray(t *testing.T) {
	mrt := models.StopLocation{Label: "Stasiun MRT Bundaran HI", Modes: models.StopModes{"TRANSIT_TRAIN"}}
	if got := mrt.GetPTOName(); got != "MRTJ" {
		t.Errorf("got %q, want MRTJ", got)
	}

	bus := models.StopLocation{Label: "Halte Monas", Modes: models.StopModes{"TRANSIT_BUS", "TRANSIT_TRAIN"}}
	if got := bus.GetPTOName(); got != "TJ" {
		t.Errorf("got %q, want TJ", got)
	}

	explicit := models.StopLocation{PTOName: "KCI", Label: "Stasiun", Modes: models.StopModes{"TRANSIT_TRAIN"}}
	if got := explicit.GetPTOName(); got != "KCI" {
		t.Errorf("got %q, want KCI (explicit pto_name must win)", got)
	}
}

func TestRouteFare_UnmarshalModesArray(t *testing.T) {
	raw := `{
		"id": "route-1",
		"origin": {"label": "Stasiun MRT Bundaran HI", "modes": ["TRANSIT_TRAIN"]},
		"destination": {"label": "Halte TransJakarta Monas", "modes": ["TRANSIT_BUS", "TRANSIT_TRAIN"]},
		"legs": [
			{
				"id": "leg-1",
				"origin": {"label": "Stasiun MRT Bundaran HI", "modes": ["TRANSIT_TRAIN"]},
				"destination": {"label": "Halte TransJakarta Monas", "modes": ["TRANSIT_BUS"]}
			}
		]
	}`

	var fare models.RouteFare
	if err := json.Unmarshal([]byte(raw), &fare); err != nil {
		t.Fatalf("route fare unmarshal failed: %v", err)
	}

	if fare.Origin.GetPTOName() != "MRTJ" {
		t.Errorf("origin PTO = %q, want MRTJ", fare.Origin.GetPTOName())
	}
	if fare.Destination.GetPTOName() != "TJ" {
		t.Errorf("destination PTO = %q, want TJ", fare.Destination.GetPTOName())
	}
	if len(fare.Legs) != 1 || len(fare.Legs[0].Origin.Modes) != 1 {
		t.Fatalf("unexpected legs: %+v", fare.Legs)
	}
}
