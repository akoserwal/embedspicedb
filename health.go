package embedspicedb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/akoserwal/embedspicedb/internal/healthhttp"
	log "github.com/akoserwal/embedspicedb/internal/logging"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// HealthStatus represents the health status of the embedded server.
type HealthStatus struct {
	Status    string            `json:"status"` // "healthy", "degraded", or "unhealthy"
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks"` // Component-level checks
	Version   string            `json:"version,omitempty"`
	Uptime    string            `json:"uptime,omitempty"`
	StartTime *time.Time        `json:"start_time,omitempty"`
}

// HealthCheck performs a comprehensive health check of the embedded server.
// It checks:
// - Server status (started/stopped)
// - gRPC connection health
// - Datastore connectivity
// - Schema availability (if schema files were provided)
func (es *EmbeddedServer) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	status := &HealthStatus{
		Status:    "", // Start with empty status, will be determined based on checks
		Timestamp: time.Now(),
		Checks:    make(map[string]string),
	}

	// Read all state under a single lock to ensure consistency
	es.mu.RLock()
	started := es.started
	startTime := es.startTime
	conn := es.conn
	ds := es.datastore
	reloader := es.reloader
	schemaFiles := es.config.SchemaFiles
	es.mu.RUnlock()

	// Check if server is started
	if !started {
		status.Checks["server"] = "not_started"
		status.Status = "unhealthy"
		return status, fmt.Errorf("server is not started")
	}
	status.Checks["server"] = "started"

	// Calculate uptime if start time is available
	if startTime != nil {
		uptime := time.Since(*startTime)
		status.Uptime = uptime.String()
		status.StartTime = startTime
	}

	// Check gRPC connection and schema in one call (avoid duplicate ReadSchema calls)
	var schemaResp *v1.ReadSchemaResponse
	var schemaErr error

	if conn == nil {
		status.Checks["grpc_connection"] = "not_available"
		status.Status = "degraded"
	} else {
		// Try to use the connection to verify it's working by reading schema once
		schemaClient := v1.NewSchemaServiceClient(conn)
		schemaResp, schemaErr = schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
		if schemaErr != nil {
			// If no schema files are configured, treat "no schema defined" as still healthy:
			// the server is up and can accept schema writes programmatically.
			if len(schemaFiles) == 0 {
				if st, ok := grpcstatus.FromError(schemaErr); ok && st.Code() == codes.NotFound {
					status.Checks["grpc_connection"] = "healthy (no schema defined)"
				} else {
					status.Checks["grpc_connection"] = fmt.Sprintf("error: %v", schemaErr)
					status.Status = "degraded"
				}
			} else {
				status.Checks["grpc_connection"] = fmt.Sprintf("error: %v", schemaErr)
				status.Status = "degraded"
			}
		} else {
			status.Checks["grpc_connection"] = "healthy"
		}
	}

	// Check datastore
	if ds == nil {
		status.Checks["datastore"] = "not_available"
		status.Status = "unhealthy"
	} else {
		// Try to get datastore statistics to verify connectivity
		// Use a short timeout to avoid blocking
		dsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err := ds.Statistics(dsCtx)
		cancel()
		if err != nil {
			// If datastore check fails, mark as degraded rather than unhealthy
			// This allows the server to be partially functional
			status.Checks["datastore"] = fmt.Sprintf("error: %v", err)
			// Only set status to degraded if not already set to unhealthy
			if status.Status != "unhealthy" {
				status.Status = "degraded"
			}
		} else {
			status.Checks["datastore"] = "healthy"
		}
	}

	// Check schema availability (if schema files were configured)
	// Reuse the schema response from the gRPC check above
	if len(schemaFiles) > 0 {
		if reloader == nil {
			status.Checks["schema"] = "reloader_not_available"
			status.Status = "degraded"
		} else if schemaErr != nil {
			status.Checks["schema"] = fmt.Sprintf("error: %v", schemaErr)
			status.Status = "degraded"
		} else if schemaResp == nil || schemaResp.SchemaText == "" {
			status.Checks["schema"] = "not_loaded"
			status.Status = "degraded"
		} else {
			status.Checks["schema"] = "loaded"
		}
	} else {
		status.Checks["schema"] = "not_configured"
	}

	// Determine overall status
	if status.Status == "unhealthy" {
		// Already set
	} else if status.Status == "degraded" {
		// Already set
	} else {
		// All checks passed
		status.Status = "healthy"
	}

	return status, nil
}

// healthCheckHandler is an HTTP handler for the health check endpoint.
func (es *EmbeddedServer) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status, err := es.HealthCheck(ctx)
	w.Header().Set("Content-Type", "application/json")
	if err != nil && status.Status == "unhealthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else if status.Status == "degraded" {
		w.WriteHeader(http.StatusOK) // 200 OK but with degraded status
	} else {
		w.WriteHeader(http.StatusOK)
	}

	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to encode health status")
	}
}

// startHealthCheckServer starts a lightweight HTTP server for health checks.
func (es *EmbeddedServer) startHealthCheckServer(ctx context.Context) error {
	if !es.config.HealthCheckEnabled {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", es.healthCheckHandler)
	mux.HandleFunc("/health", es.healthCheckHandler) // Alias for /healthz

	srv, err := healthhttp.Start(es.config.HealthCheckAddress, mux)
	if err != nil {
		return err
	}
	es.healthSrv = srv
	log.Ctx(ctx).Info().
		Str("address", srv.Addr()).
		Msg("health check server started")

	return nil
}

// stopHealthCheckServer stops the health check HTTP server.
func (es *EmbeddedServer) stopHealthCheckServer(ctx context.Context) error {
	if es.healthSrv == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := es.healthSrv.Shutdown(shutdownCtx); err != nil {
		log.Ctx(ctx).Warn().Err(err).Msg("error shutting down health check server")
		return err
	}
	es.healthSrv = nil

	log.Ctx(ctx).Info().Msg("health check server stopped")
	return nil
}
