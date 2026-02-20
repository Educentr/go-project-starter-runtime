package rest

import (
	"context"
	"time"
)

// ServerConfig holds HTTP server configuration values.
type ServerConfig struct {
	IP                string
	Port              string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	ReadHeaderTimeout time.Duration
}

// ServerConfigProvider abstracts server configuration retrieval.
// Implementations can use OnlineConf, env vars, static config, etc.
type ServerConfigProvider interface {
	// GetServerConfig returns the server configuration.
	GetServerConfig(ctx context.Context, serverName, appName string) (ServerConfig, error)

	// SubscribeTimeoutChanges registers a callback for timeout changes.
	// The callback is invoked when any timeout value changes.
	// Returns nil if subscriptions are not supported.
	SubscribeTimeoutChanges(ctx context.Context, serverName, appName string, callback func()) error
}

// HandlerTimeoutProvider resolves handler-level timeouts.
type HandlerTimeoutProvider interface {
	// ResolveHandlerTimeout returns the timeout for a specific handler path.
	// Returns 0 if no timeout is configured.
	ResolveHandlerTimeout(ctx context.Context, transportName, appName, pathKey string) time.Duration
}

// SecurityConfigProvider manages auth/CSRF toggle.
type SecurityConfigProvider interface {
	// IsHTTPAuthEnabled returns whether HTTP authentication is enabled.
	IsHTTPAuthEnabled(ctx context.Context) (bool, error)

	// IsCSRFEnabled returns whether CSRF protection is enabled.
	IsCSRFEnabled(ctx context.Context) (bool, error)
}
