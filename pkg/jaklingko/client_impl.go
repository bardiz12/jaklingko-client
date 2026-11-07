package jaklingko

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bardiz12/jaklingko-client/pkg/crypto"
	"github.com/bardiz12/jaklingko-client/pkg/models"
)

type clientImpl struct {
	mu           sync.RWMutex
	httpClient   *http.Client
	baseURL      string
	cipherSalt   string
	detailSalt   string
	cipher       *crypto.CipherEngine
	session      *models.Session
	sessionPath  string
	appVersion   string
	manufacturer string
	model        string
	defaultUser  string
	defaultPass  string
}

// GetSession returns the current active session (thread-safe copy)
func (c *clientImpl) GetSession() *models.Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// HasValidSession returns true if there is an active access token
func (c *clientImpl) HasValidSession() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session != nil && c.session.AccessToken != ""
}

// SetSession updates the active session in memory and persists it to disk
func (c *clientImpl) SetSession(sess *models.Session) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = sess
	return SaveSession(c.sessionPath, sess)
}

// ClearSession clears the current session in memory and deletes the session file
func (c *clientImpl) ClearSession() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.session = nil
	return ClearSession(c.sessionPath)
}

// DoRequest performs an HTTP request with device fingerprint headers, encryption,
// and automatic response payload decryption.
func (c *clientImpl) DoRequest(ctx context.Context, method, endpoint string, reqBody interface{}, encryptBody bool) ([]byte, int, error) {
	fullURL := c.baseURL + "/" + strings.TrimLeft(endpoint, "/")

	var bodyReader io.Reader
	var isEncrypted bool

	if reqBody != nil {
		rawJSON, err := json.Marshal(reqBody)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request body: %w", err)
		}

		if encryptBody {
			encPayload, err := c.cipher.EncryptPayload(string(rawJSON))
			if err != nil {
				return nil, 0, fmt.Errorf("failed to encrypt request payload: %w", err)
			}
			bodyReader = bytes.NewBufferString(encPayload)
			isEncrypted = true
		} else {
			bodyReader = bytes.NewReader(rawJSON)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create http request: %w", err)
	}

	// Standard JakLingko Android headers discovered via reverse engineering
	httpReq.Header.Set("App-Version", c.appVersion)
	httpReq.Header.Set("Device-Manufacturer", c.manufacturer)
	httpReq.Header.Set("Device-Model", c.model)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	httpReq.Header.Set("Pragma", "no-cache")
	httpReq.Header.Set("Expires", "0")

	if reqBody != nil {
		httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	if isEncrypted {
		httpReq.Header.Set("X-Encrypted", "true")
	}

	// Inject Bearer token if user has an active session
	if c.HasValidSession() {
		c.mu.RLock()
		token := c.session.AccessToken
		c.mu.RUnlock()
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("http request execution failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	// Automatic response payload decryption
	decryptedBytes, err := c.cipher.DecryptPayload(respBytes)
	if err != nil {
		// Fallback to raw response bytes if response was unencrypted
		decryptedBytes = respBytes
	}

	// Handle 401 Unauthorized token expiration
	if resp.StatusCode == http.StatusUnauthorized && !strings.Contains(endpoint, "auth/login") {
		_ = c.ClearSession()
		return decryptedBytes, resp.StatusCode, errors.New("unauthorized: session token expired or invalid (HTTP 401)")
	}

	// Handle backend profile service disconnect (PROFILE_CONNECT_ERROR)
	if resp.StatusCode == http.StatusInternalServerError && strings.Contains(string(decryptedBytes), "PROFILE_CONNECT_ERROR") {
		_ = c.ClearSession()
		return decryptedBytes, resp.StatusCode, errors.New("backend profile service disconnected (PROFILE_CONNECT_ERROR, HTTP 500); please re-login")
	}

	return decryptedBytes, resp.StatusCode, nil
}
