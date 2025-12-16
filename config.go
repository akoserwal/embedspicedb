package embedspicedb

import (
	"time"
)

// Config holds configuration for embedded SpiceDB server with hot reload.
type Config struct {
	// SchemaFiles contains paths to schema files to watch for changes.
	// Supported formats: .zed files (plain schema text) or .yaml/.yml files (validation files).
	SchemaFiles []string

	// GRPCAddress is the address for the gRPC server (e.g., ":50051").
	// If empty, defaults to ":50051".
	GRPCAddress string

	// HTTPEnabled enables the HTTP gateway.
	HTTPEnabled bool

	// HTTPAddress is the address for the HTTP gateway (e.g., ":8443").
	// Only used if HTTPEnabled is true.
	HTTPAddress string

	// PresharedKey is the authentication key for API requests.
	// If empty, defaults to "dev-key" for development.
	PresharedKey string

	// WatchDebounce is the debounce interval for file changes.
	// This prevents rapid reloads when files are being edited.
	// If zero, defaults to 500ms.
	WatchDebounce time.Duration

	// RevisionQuantization is the interval for quantizing revisions.
	// If zero, defaults to 5 seconds.
	RevisionQuantization time.Duration

	// GCWindow is the garbage collection window.
	// If zero, defaults to 24 hours.
	GCWindow time.Duration

	// WatchBufferLength is the buffer length for watch operations.
	// If zero, uses datastore default.
	WatchBufferLength uint16

	// DatastoreType specifies the type of datastore to use.
	// Options: "memdb" (default), "postgres", "mysql"
	// If "postgres" or "mysql", DatastoreURI must be provided.
	DatastoreType string

	// DatastoreURI is the connection URI for persistent datastores.
	// Required if DatastoreType is "postgres" or "mysql".
	// For PostgreSQL: "postgres://user:password@host:port/database?sslmode=disable"
	// For MySQL: "user:password@tcp(host:port)/database?parseTime=true"
	DatastoreURI string

	// HealthCheckEnabled enables the health check HTTP endpoint.
	// If true, a health check endpoint will be available at /healthz.
	// Defaults to false.
	HealthCheckEnabled bool

	// HealthCheckAddress is the address for the health check HTTP server.
	// If empty and HealthCheckEnabled is true, defaults to "127.0.0.1:0" (random free port).
	// This is separate from the HTTP gateway and provides a lightweight health check endpoint.
	HealthCheckAddress string
}

// DefaultConfig returns a Config with sensible defaults for development.
func DefaultConfig() Config {
	return Config{
		SchemaFiles:          []string{},
		GRPCAddress:          ":50051",
		HTTPEnabled:          false,
		HTTPAddress:          ":8443",
		PresharedKey:         "dev-key",
		WatchDebounce:        500 * time.Millisecond,
		RevisionQuantization: 5 * time.Second,
		GCWindow:             24 * time.Hour,
		WatchBufferLength:    0,       // Use datastore default
		DatastoreType:        "memdb", // Default to in-memory for development
		DatastoreURI:         "",
		HealthCheckEnabled:   false,
		HealthCheckAddress:   "127.0.0.1:0",
	}
}

// WithDefaults applies defaults to unset fields.
func (c *Config) WithDefaults() {
	if c.GRPCAddress == "" {
		c.GRPCAddress = ":50051"
	}
	if c.PresharedKey == "" {
		c.PresharedKey = "dev-key"
	}
	if c.WatchDebounce == 0 {
		c.WatchDebounce = 500 * time.Millisecond
	}
	if c.RevisionQuantization == 0 {
		c.RevisionQuantization = 5 * time.Second
	}
	if c.GCWindow == 0 {
		c.GCWindow = 24 * time.Hour
	}
	if c.HTTPAddress == "" && c.HTTPEnabled {
		c.HTTPAddress = ":8443"
	}
	if c.DatastoreType == "" {
		c.DatastoreType = "memdb"
	}
	if c.HealthCheckAddress == "" && c.HealthCheckEnabled {
		c.HealthCheckAddress = "127.0.0.1:0"
	}
}
