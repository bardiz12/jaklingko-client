package models

// ApiResult represents the generic JSON response structure from the JakLingko backend
type ApiResult[T any] struct {
	StatusCode   int      `json:"statusCode"`
	Message      string   `json:"message"`
	TitleMessage string   `json:"title_message,omitempty"`
	Error        []string `json:"error,omitempty"`
	Data         T        `json:"data"`
}

// LoginRequest is the body for POST auth/v3/1/auth/login/user
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthData holds the access token returned upon successful login
type AuthData struct {
	AccessToken string `json:"access_token"`
	HasEmail    bool   `json:"has_email"`
	HasPassword bool   `json:"has_password"`
	HasPhone    bool   `json:"has_phone"`
}
