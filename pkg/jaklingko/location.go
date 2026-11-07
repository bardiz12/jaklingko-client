package jaklingko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/bardiz12/jaklingko-client/pkg/models"
)

// SearchLocation searches for transit stations/stops by name using the position endpoint
// (route/v4/route/position) with fallback to route/v4/route/location/name
func (c *clientImpl) SearchLocation(ctx context.Context, query string, limit int) ([]models.StopLocation, error) {
	if limit <= 0 {
		limit = 10
	}

	// 1. Primary: route/v4/route/position (Accurate coordinates used by official app)
	posParams := url.Values{}
	posParams.Set("query", query)
	posParams.Set("country", "ID")
	posParams.Set("limit", strconv.Itoa(limit))
	posParams.Set("page", "1")

	posEndpoint := "route/v4/route/position?" + posParams.Encode()
	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodGet, posEndpoint, nil, false)
	if err == nil && statusCode == http.StatusOK {
		var result models.ApiResult[[]models.StopLocation]
		if err := json.Unmarshal(respBytes, &result); err == nil && len(result.Data) > 0 {
			return result.Data, nil
		}
	}

	// 2. Fallback: route/v4/route/location/name
	fallbackParams := url.Values{}
	fallbackParams.Set("query", query)
	fallbackParams.Set("page", "1")
	fallbackParams.Set("view", strconv.Itoa(limit))

	fallbackEndpoint := "route/v4/route/location/name?" + fallbackParams.Encode()
	fbBytes, fbStatus, fbErr := c.DoRequest(ctx, http.MethodGet, fallbackEndpoint, nil, false)
	if fbErr != nil {
		if err != nil {
			return nil, fmt.Errorf("location search failed: %w", err)
		}
		return nil, fmt.Errorf("location search fallback failed: %w", fbErr)
	}

	if fbStatus != http.StatusOK {
		return nil, fmt.Errorf("location search returned HTTP %d: %s", fbStatus, string(fbBytes))
	}

	var fbResult models.ApiResult[[]models.StopLocation]
	if err := json.Unmarshal(fbBytes, &fbResult); err != nil {
		return nil, fmt.Errorf("failed to unmarshal location search response: %w (raw: %s)", err, string(fbBytes))
	}

	return fbResult.Data, nil
}
