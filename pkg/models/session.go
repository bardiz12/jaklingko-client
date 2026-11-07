package models

import "time"

// Session stores persistent user credentials and tokens
type Session struct {
	AccessToken string    `json:"access_token"`
	Username    string    `json:"username"`
	SavedAt     time.Time `json:"saved_at"`
}

// IsValid checks if the session contains an access token
func (s *Session) IsValid() bool {
	return s != nil && s.AccessToken != ""
}
