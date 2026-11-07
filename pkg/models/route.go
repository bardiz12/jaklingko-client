package models

// RouteFare represents the comprehensive route calculation and fare details
type RouteFare struct {
	ID                string             `json:"id"`
	TotalAmount       int                `json:"totalAmount"`
	TotalAmountMarked int                `json:"totalAmountMarked"`
	PTOCost           int                `json:"ptoCost"`
	GrabCost          int                `json:"grabCost"`
	CostsFixed        int                `json:"costs_fixed"`
	Duration          int                `json:"duration"` // in seconds
	GeneralDuration   string             `json:"general_duration"`
	Distance          int                `json:"distance"` // in meters
	GeneralDistance   float64            `json:"general_distance"`
	Origin            StopLocation       `json:"origin"`
	Destination       StopLocation       `json:"destination"`
	Legs              []RouteLeg         `json:"legs"`
	ServiceIDs        []string           `json:"serviceIds,omitempty"`
	BookingPayload    *BookTicketRequest `json:"booking_payload,omitempty"`
}

// Agency represents transit operator details
type Agency struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
	URL  string `json:"url"`
}

// RouteLeg represents an individual transit leg/segment (bus, train, walk, etc.)
type RouteLeg struct {
	ID                 string       `json:"id"`
	Mode               string       `json:"mode"`      // TRANSIT_BUS, TRANSIT_TRAMWAY, FOOT, etc.
	ShowMode           string       `json:"show_mode"` // TRANSJAKARTA, MRT, LRT, KRL, JALAN_KAKI
	Color              string       `json:"color"`
	Duration           int          `json:"duration"`
	Distance           int          `json:"distance"`
	Polyline           string       `json:"polyline"`
	Agency             *Agency      `json:"agency,omitempty"`
	Line               *TransitLine `json:"line,omitempty"`
	Origin             StopLocation `json:"origin"`
	Destination        StopLocation `json:"destination"`
	GrabCostEstimation int          `json:"grab_cost_estimation,omitempty"`
}

// TransitLine holds the line/route name and code
type TransitLine struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	LongName  string `json:"longName"`
	Desc      string `json:"desc"`
	RouteType string `json:"routeType"`
	Direction string `json:"direction,omitempty"`
}
