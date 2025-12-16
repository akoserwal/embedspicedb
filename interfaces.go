package embedspicedb

import (
	"context"

	"google.golang.org/grpc"
)

// Server is the public surface area of an embedded SpiceDB server.
// Keeping this interface small makes it easier to remain compatible with SpiceDB over time.
type Server interface {
	Start(ctx context.Context) error
	Stop() error

	// Client returns a gRPC connection to the embedded SpiceDB API.
	Client(ctx context.Context) (*grpc.ClientConn, error)

	// ReloadSchema forces a schema reload from configured schema files.
	ReloadSchema(ctx context.Context) error

	// OnSchemaReloaded registers a callback invoked after each reload attempt.
	OnSchemaReloaded(callback func(error))

	// HealthCheck returns detailed component health.
	HealthCheck(ctx context.Context) (*HealthStatus, error)

	// HealthCheckHTTPAddr returns the bound address for the HTTP health endpoint, if enabled.
	HealthCheckHTTPAddr() string
}
