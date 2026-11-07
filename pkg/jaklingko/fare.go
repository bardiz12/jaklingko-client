package jaklingko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/bardiz12/jaklingko-client/pkg/models"
)

// FareSearchOptions holds optional filters for route and fare searches
type FareSearchOptions struct {
	Sort     string // "cheapest", "fastest", "least_transfer"
	Datasets string // e.g. "TJ,MRT,LRT,COMMUTER" or empty for all
	Operator string // e.g. "all"
	Date     string // ISO datetime or YYYY-MM-DD HH:mm:ss, defaults to now
	Page     int
	View     int
}

// SearchTicketFare queries integrated multimodal transit routes and computes fares
func (c *clientImpl) SearchTicketFare(ctx context.Context, origin, destination models.StopLocation, opts *FareSearchOptions) ([]models.RouteFare, error) {
	if opts == nil {
		opts = &FareSearchOptions{}
	}

	if opts.View <= 0 {
		opts.View = 10
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.Sort == "" {
		opts.Sort = "cheapest"
	}
	if opts.Date == "" {
		opts.Date = time.Now().Format("15:04:05")
	}

	params := url.Values{}
	params.Set("origin", fmt.Sprintf("%f,%f", origin.GetLat(), origin.GetLng()))
	params.Set("destination", fmt.Sprintf("%f,%f", destination.GetLat(), destination.GetLng()))
	params.Set("origin_label", origin.GetDisplayName())
	params.Set("destination_label", destination.GetDisplayName())
	params.Set("datasets", opts.Datasets)
	params.Set("sort", opts.Sort)
	params.Set("route_mode", "true")
	params.Set("date", opts.Date)
	params.Set("operator", opts.Operator)
	params.Set("view", strconv.Itoa(opts.View))
	params.Set("page", strconv.Itoa(opts.Page))

	endpoint := "route/v4/1/route?" + params.Encode()

	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodGet, endpoint, nil, false)
	if err != nil {
		return nil, fmt.Errorf("fare search request failed: %w", err)
	}

	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("fare search returned HTTP %d: %s", statusCode, string(respBytes))
	}

	var result models.ApiResult[[]models.RouteFare]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fare search response: %w (raw: %s)", err, string(respBytes))
	}

	for i := range result.Data {
		if result.Data[i].Origin.GetDisplayName() == "" {
			result.Data[i].Origin = origin
		}
		if result.Data[i].Destination.GetDisplayName() == "" {
			result.Data[i].Destination = destination
		}
		bp := BuildBookingPayload(result.Data[i], PaymentChannelQRIS)
		result.Data[i].BookingPayload = &bp
	}

	return result.Data, nil
}
