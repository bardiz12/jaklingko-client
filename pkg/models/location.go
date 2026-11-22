package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// StopModes holds the transport modes served by a stop. The JakLingko API
// returns the "modes" field either as a single string ("TRANSIT_BUS") or as an
// array (["TRANSIT_BUS", "TRANSIT_TRAIN"]), so it needs a tolerant unmarshaler.
// A plain string field fails on array responses and breaks every enclosing
// StopLocation, including route and fare search results.
type StopModes []string

// UnmarshalJSON accepts a string, an array of strings, or null.
func (m *StopModes) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*m = nil
		return nil
	}

	if trimmed[0] == '"' {
		var single string
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return fmt.Errorf("modes: %w", err)
		}
		if single == "" {
			*m = nil
			return nil
		}
		*m = StopModes{single}
		return nil
	}

	var list []string
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return fmt.Errorf("modes must be a string or an array of strings: %w", err)
	}
	*m = list
	return nil
}

// MarshalJSON keeps the historical single-string representation when exactly
// one mode is present, emits an array for multiple modes, and an empty string
// when the field was absent (nil).
func (m StopModes) MarshalJSON() ([]byte, error) {
	switch {
	case len(m) == 1:
		return json.Marshal(m[0])
	case m == nil:
		return json.Marshal("")
	default:
		return json.Marshal([]string(m))
	}
}

// String joins the modes into a single search-friendly string.
func (m StopModes) String() string {
	return strings.Join(m, " ")
}

// LocationPoint represents geo coordinates (lat, lng)
type LocationPoint struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// StopLocation represents a transit stop/station found via location search
type StopLocation struct {
	Label       string        `json:"label"`
	Name        string        `json:"name"`
	Address     string        `json:"address"`
	Locality    string        `json:"locality"`
	PTOName     string        `json:"pto_name"` // e.g. TJ, MRT, LRT, KCI
	Modes       StopModes     `json:"modes"`    // e.g. "TRANSIT_BUS" or ["TRANSIT_BUS", "TRANSIT_TRAIN"]
	StopCode    string        `json:"stop_code"`
	StopCodeAlt string        `json:"stopCode"`
	StopID      interface{}   `json:"stop_id"`
	LykoStopID  string        `json:"lyko_stop_id"`
	LykoStopAlt string        `json:"lykoStopId"`
	Latitude    float64       `json:"latitude"`
	Longitude   float64       `json:"longitude"`
	Location    LocationPoint `json:"location"`
}

// GetLat returns the latitude coordinate
func (s *StopLocation) GetLat() float64 {
	if s.Latitude != 0 {
		return s.Latitude
	}
	return s.Location.Lat
}

// GetLng returns the longitude coordinate
func (s *StopLocation) GetLng() float64 {
	if s.Longitude != 0 {
		return s.Longitude
	}
	return s.Location.Lng
}

// GetDisplayName returns the most suitable display name for the stop
func (s *StopLocation) GetDisplayName() string {
	if s.Label != "" {
		return s.Label
	}
	if s.Name != "" {
		return s.Name
	}
	if s.Locality != "" {
		return s.Locality
	}
	return s.Address
}

// GetPTOName infers or returns the transit operator name
func (s *StopLocation) GetPTOName() string {
	if s.PTOName != "" {
		return s.PTOName
	}
	lbl := strings.ToUpper(s.Label + " " + s.Modes.String())
	if strings.Contains(lbl, "MRT") {
		return "MRTJ"
	}
	if strings.Contains(lbl, "KCI") || strings.Contains(lbl, "KRL") || strings.Contains(lbl, "COMMUTER") {
		return "KCI"
	}
	if strings.Contains(lbl, "LRT") {
		return "LRTJ"
	}
	if strings.Contains(lbl, "BRT") || strings.Contains(lbl, "MIKROTRANS") || strings.Contains(lbl, "TRANSJAKARTA") || strings.Contains(lbl, "TJ") || strings.Contains(lbl, "BUS") {
		return "TJ"
	}
	return ""
}
