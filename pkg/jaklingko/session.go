package jaklingko

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/bardiz12/jaklingko-client/pkg/models"
)

// DefaultSessionFile is the fallback file path for storing user sessions
const DefaultSessionFile = ".session.json"

// LoadSession loads a previously saved session from disk
func LoadSession(filePath string) (*models.Session, error) {
	if filePath == "" {
		filePath = DefaultSessionFile
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var sess models.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}

	if !sess.IsValid() {
		return nil, errors.New("saved session is invalid or empty")
	}

	return &sess, nil
}

// SaveSession securely writes user session data to disk with restricted permissions (0600)
func SaveSession(filePath string, sess *models.Session) error {
	if filePath == "" {
		filePath = DefaultSessionFile
	}

	if sess == nil || !sess.IsValid() {
		return errors.New("cannot save empty or invalid session")
	}

	if sess.SavedAt.IsZero() {
		sess.SavedAt = time.Now()
	}

	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0600)
}

// ClearSession removes the session file from disk
func ClearSession(filePath string) error {
	if filePath == "" {
		filePath = DefaultSessionFile
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
