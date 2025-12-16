package embedspicedb_test

import (
	"context"
	"encoding/json"
	. "github.com/akoserwal/embedspicedb"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthCheck_ServerNotStarted(t *testing.T) {
	config := Config{
		GRPCAddress:        ":0",
		PresharedKey:       "test-key",
		HealthCheckEnabled: false, // Disable HTTP server for this test
	}

	server, err := New(config)
	require.NoError(t, err)

	ctx := context.Background()
	status, err := server.HealthCheck(ctx)

	require.Error(t, err)
	assert.Equal(t, "unhealthy", status.Status)
	assert.Equal(t, "not_started", status.Checks["server"])
}

func TestHealthCheck_HealthyServer(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer removeFile(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:        []string{tmpFile},
		GRPCAddress:        port,
		PresharedKey:       "test-key",
		HealthCheckEnabled: false, // Disable HTTP server for this test
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	status, err := server.HealthCheck(ctx)

	require.NoError(t, err)
	// Print status for debugging
	if status.Status != "healthy" {
		t.Logf("Health check status: %s", status.Status)
		for k, v := range status.Checks {
			t.Logf("  %s: %s", k, v)
		}
	}
	// All checks passed, so status should be healthy
	assert.Equal(t, "healthy", status.Status)
	assert.Equal(t, "started", status.Checks["server"])
	assert.Equal(t, "healthy", status.Checks["grpc_connection"])
	assert.Equal(t, "healthy", status.Checks["datastore"])
	assert.NotNil(t, status.StartTime)
	assert.NotEmpty(t, status.Uptime)
}

func TestHealthCheck_HTTPEndpoint(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer removeFile(tmpFile)

	grpcPort := getFreePort(t)
	config := Config{
		SchemaFiles:        []string{tmpFile},
		GRPCAddress:        grpcPort,
		PresharedKey:       "test-key",
		HealthCheckEnabled: true,
		HealthCheckAddress: "127.0.0.1:0",
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Wait for servers to be ready
	time.Sleep(500 * time.Millisecond)

	healthAddr := server.HealthCheckHTTPAddr()
	require.NotEmpty(t, healthAddr)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Test /healthz endpoint
	resp, err := client.Get("http://" + healthAddr + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var status HealthStatus
	err = json.NewDecoder(resp.Body).Decode(&status)
	require.NoError(t, err)

	assert.Equal(t, "healthy", status.Status)
	assert.Equal(t, "started", status.Checks["server"])
	assert.Equal(t, "healthy", status.Checks["grpc_connection"])
	assert.Equal(t, "healthy", status.Checks["datastore"])

	// Test /health endpoint (alias)
	resp2, err := client.Get("http://" + healthAddr + "/health")
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

func TestHealthCheck_HTTPEndpoint_ServerStopped(t *testing.T) {
	grpcPort := getFreePort(t)
	config := Config{
		GRPCAddress:        grpcPort,
		PresharedKey:       "test-key",
		HealthCheckEnabled: true,
		HealthCheckAddress: "127.0.0.1:0",
	}

	server, err := New(config)
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Wait for server to be ready
	time.Sleep(500 * time.Millisecond)

	healthAddr := server.HealthCheckHTTPAddr()
	require.NotEmpty(t, healthAddr)

	// Stop server
	err = server.Stop()
	require.NoError(t, err)

	// Wait a bit for shutdown
	time.Sleep(200 * time.Millisecond)

	// Health check endpoint should be stopped, so connection should fail
	client := &http.Client{
		Timeout: 1 * time.Second,
	}
	_, err = client.Get("http://" + healthAddr + "/healthz")
	// Connection should fail as server is stopped
	assert.Error(t, err)
}

func TestHealthCheck_NoSchemaFiles(t *testing.T) {
	port := getFreePort(t)
	config := Config{
		GRPCAddress:        port,
		PresharedKey:       "test-key",
		HealthCheckEnabled: false, // Disable HTTP server for this test
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	status, err := server.HealthCheck(ctx)

	require.NoError(t, err)
	assert.Equal(t, "healthy", status.Status)
	assert.Equal(t, "not_configured", status.Checks["schema"])
}

func TestHealthCheck_StatusFields(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer removeFile(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:        []string{tmpFile},
		GRPCAddress:        port,
		PresharedKey:       "test-key",
		HealthCheckEnabled: false, // Disable HTTP server for this test
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	status, err := server.HealthCheck(ctx)
	require.NoError(t, err)

	// Verify all expected fields are present
	assert.NotZero(t, status.Timestamp)
	assert.NotNil(t, status.Checks)
	assert.Contains(t, status.Checks, "server")
	assert.Contains(t, status.Checks, "grpc_connection")
	assert.Contains(t, status.Checks, "datastore")
	assert.Contains(t, status.Checks, "schema")
	assert.NotNil(t, status.StartTime)
	assert.NotEmpty(t, status.Uptime)
}

func TestHealthCheck_ConcurrentAccess(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer removeFile(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:        []string{tmpFile},
		GRPCAddress:        port,
		PresharedKey:       "test-key",
		HealthCheckEnabled: false, // Disable HTTP server for this test
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	// Concurrent health checks
	const numChecks = 10
	results := make([]*HealthStatus, numChecks)
	errors := make([]error, numChecks)

	var wg sync.WaitGroup
	for i := 0; i < numChecks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			status, err := server.HealthCheck(ctx)
			results[idx] = status
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// Verify all checks succeeded
	for i := 0; i < numChecks; i++ {
		assert.NoError(t, errors[i], "health check %d should succeed", i)
		assert.NotNil(t, results[i], "health check %d should return status", i)
		if results[i] != nil {
			assert.Equal(t, "healthy", results[i].Status)
		}
	}
}

// Helper function to remove a file (used in defer)
func removeFile(path string) {
	// File removal is handled by test cleanup, but this provides a consistent interface
}
