package models

import "strings"

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
	Modes       string        `json:"modes"`    // e.g. TRANSIT_BUS, TRANSIT_TRAIN
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
	lbl := strings.ToUpper(s.Label + " " + s.Modes)
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
