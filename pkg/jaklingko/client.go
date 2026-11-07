package jaklingko

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bardiz12/jaklingko-client/pkg/crypto"
	"github.com/bardiz12/jaklingko-client/pkg/models"
)

const (
	// DefaultBaseURL is the production JakLingko API Gateway endpoint
	DefaultBaseURL = "https://api-sapps.jaklingkoindonesia.co.id"

	// DefaultTimeout is the default HTTP request timeout
	DefaultTimeout = 45 * time.Second

	// DefaultAppVersion is the target Android application version
	DefaultAppVersion = "1.6.22"

	// PaymentChannelQRIS is the channel code for QRIS in JakLingko
	PaymentChannelQRIS = "4"
)

// Client represents the primary interface for interacting with JakLingko services.
// Each Client instance encapsulates an isolated user session, token lifecycle,
// and HTTP connection state. To manage multiple accounts simultaneously,
// instantiate separate Client instances using New().
type Client interface {
	// Authentication & Session
	Login(ctx context.Context, username, password string) (*models.AuthData, error)
	Logout(ctx context.Context) error
	GetSession() *models.Session
	HasValidSession() bool
	SetSession(sess *models.Session) error
	ClearSession() error

	// Transit & Routing
	SearchLocation(ctx context.Context, query string, limit int) ([]models.StopLocation, error)
	SearchTicketFare(ctx context.Context, origin, destination models.StopLocation, opts *FareSearchOptions) ([]models.RouteFare, error)

	// Ticketing & Payments
	BookTicket(ctx context.Context, req models.BookTicketRequest) (*models.BookingResult, error)
	GetPaymentChannels(ctx context.Context) ([]models.PaymentChannelInfo, error)
	GetPendingTickets(ctx context.Context) ([]models.PendingTicket, error)
	GetActiveTickets(ctx context.Context) ([]models.ActiveTicketItem, error)
	GetTicketDetail(ctx context.Context, ticketLongID string) (*models.TicketDetailData, error)
	GenerateGateQR(ctx context.Context, ticketID, legID int) (*models.GateQRResult, error)
	GenerateGateQRForTicket(ctx context.Context, ticketLongID string, legID int) ([]models.GateLegQRInfo, error)

	// Low-level HTTP transport with transparent encryption/decryption
	DoRequest(ctx context.Context, method, endpoint string, reqBody interface{}, encryptBody bool) ([]byte, int, error)
}

// Option configures a Client instance
type Option func(*clientImpl)

// WithBaseURL overrides the default gateway URL
func WithBaseURL(url string) Option {
	return func(c *clientImpl) {
		if url != "" {
			c.baseURL = strings.TrimRight(url, "/")
		}
	}
}

// WithCipherSalt overrides the cryptographic salt used for general request/response payloads
func WithCipherSalt(salt string) Option {
	return func(c *clientImpl) {
		if salt != "" {
			c.cipherSalt = salt
		}
	}
}

// WithDetailSalt overrides the cryptographic salt used for active ticket detail payloads
func WithDetailSalt(salt string) Option {
	return func(c *clientImpl) {
		if salt != "" {
			c.detailSalt = salt
		}
	}
}

// WithSessionFile sets the isolated session file path for this client instance
func WithSessionFile(path string) Option {
	return func(c *clientImpl) {
		c.sessionPath = path
	}
}

// WithHTTPClient provides a custom http.Client for this client instance
func WithHTTPClient(hc *http.Client) Option {
	return func(c *clientImpl) {
		c.httpClient = hc
	}
}

// WithDeviceInfo sets custom Android device fingerprint headers
func WithDeviceInfo(appVersion, manufacturer, model string) Option {
	return func(c *clientImpl) {
		if appVersion != "" {
			c.appVersion = appVersion
		}
		if manufacturer != "" {
			c.manufacturer = manufacturer
		}
		if model != "" {
			c.model = model
		}
	}
}

// WithCredentials configures default credentials for automatic authentication
func WithCredentials(username, password string) Option {
	return func(c *clientImpl) {
		c.defaultUser = username
		c.defaultPass = password
	}
}

// New creates and initializes a new Client instance.
// Each returned instance is completely isolated, maintaining its own session
// file, in-memory tokens, cipher engine, and HTTP client.
func New(opts ...Option) Client {
	c := &clientImpl{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		baseURL:      DefaultBaseURL,
		sessionPath:  DefaultSessionFile,
		appVersion:   DefaultAppVersion,
		manufacturer: "Google",
		model:        "Pixel 7",
	}

	for _, opt := range opts {
		opt(c)
	}

	// Environment variable overrides with fallback to defaults
	if envURL := os.Getenv("JAKLINGKO_BASE_URL"); envURL != "" && c.baseURL == DefaultBaseURL {
		c.baseURL = strings.TrimRight(envURL, "/")
	}
	if envCipherSalt := os.Getenv("JAKLINGKO_CIPHER_SALT"); envCipherSalt != "" && c.cipherSalt == "" {
		c.cipherSalt = envCipherSalt
	}
	if envDetailSalt := os.Getenv("JAKLINGKO_DETAIL_SALT"); envDetailSalt != "" && c.detailSalt == "" {
		c.detailSalt = envDetailSalt
	}

	// Initialize cipher engine with configured salt (or default fallback)
	c.cipher = crypto.NewCipherEngine(c.cipherSalt)

	// Attempt to load existing saved session for this client instance
	if sess, err := LoadSession(c.sessionPath); err == nil && sess.IsValid() {
		c.session = sess
	}

	return c
}
