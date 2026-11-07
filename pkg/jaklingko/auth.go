package jaklingko

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bardiz12/jaklingko-client/pkg/models"
)

// NormalizePhoneNumber formats an Indonesian phone number into the required international "62..." format
func NormalizePhoneNumber(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, "-", "")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.TrimPrefix(phone, "+")
	if strings.HasPrefix(phone, "0") {
		return "62" + strings.TrimPrefix(phone, "0")
	}
	if !strings.HasPrefix(phone, "62") && len(phone) > 0 {
		return "62" + phone
	}
	return phone
}

// Login authenticates against POST auth/v3/1/auth/login/user.
// It automatically normalizes the phone number format, encrypts credentials,
// and persists the resulting access token into this client instance's session storage.
func (c *clientImpl) Login(ctx context.Context, username, password string) (*models.AuthData, error) {
	normUser := NormalizePhoneNumber(username)
	normPass := strings.TrimSpace(password)
	normPass = strings.ReplaceAll(normPass, " ", "")

	req := models.LoginRequest{
		Username: normUser,
		Password: normPass,
	}

	respBytes, statusCode, err := c.DoRequest(ctx, http.MethodPost, "auth/v3/1/auth/login/user", req, true)
	if err != nil {
		return nil, fmt.Errorf("login request failed: %w", err)
	}

	var result models.ApiResult[models.AuthData]
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal login response: %w (raw: %s)", err, string(respBytes))
	}

	isSuccess := (statusCode == http.StatusOK || statusCode == http.StatusCreated) &&
		(result.StatusCode == http.StatusOK || result.StatusCode == http.StatusCreated || result.StatusCode == 0)

	if !isSuccess {
		msg := result.Message
		if len(result.Error) > 0 {
			msg += fmt.Sprintf(" (%s)", strings.Join(result.Error, ", "))
		}
		if statusCode == http.StatusForbidden || (len(result.Error) > 0 && result.Error[0] == "AUTH_BLOCK_TEMP") {
			return nil, fmt.Errorf("login temporarily blocked (AUTH_BLOCK_TEMP / HTTP 403). Your account or IP has been rate-limited due to repeated attempts. Please wait 5-10 minutes before retrying")
		}
		return nil, fmt.Errorf("login failed (HTTP %d, Code %d): %s", statusCode, result.StatusCode, msg)
	}

	if result.Data.AccessToken == "" {
		return nil, fmt.Errorf("login response did not contain an access token (raw: %s)", string(respBytes))
	}

	// Persist session into this instance's isolated session storage
	sess := &models.Session{
		AccessToken: result.Data.AccessToken,
		Username:    normUser,
		SavedAt:     time.Now(),
	}

	if err := c.SetSession(sess); err != nil {
		return nil, fmt.Errorf("login succeeded but failed to persist session: %w", err)
	}

	return &result.Data, nil
}

// Logout clears the current session in memory and removes the session file
func (c *clientImpl) Logout(ctx context.Context) error {
	return c.ClearSession()
}
