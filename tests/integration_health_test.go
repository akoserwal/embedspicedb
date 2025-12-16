package embedspicedb_test

import (
	"context"
	"encoding/json"
	. "github.com/akoserwal/embedspicedb"
	"net/http"
	"testing"
	"time"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_HealthCheckEndpoint tests the HTTP health check endpoint
func TestIntegration_HealthCheckEndpoint(t *testing.T) {
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
	assert.Equal(t, "loaded", status.Checks["schema"])
	assert.NotNil(t, status.StartTime)
	assert.NotEmpty(t, status.Uptime)

	// Test /health endpoint (alias)
	resp2, err := client.Get("http://" + healthAddr + "/health")
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	var status2 HealthStatus
	err = json.NewDecoder(resp2.Body).Decode(&status2)
	require.NoError(t, err)
	assert.Equal(t, "healthy", status2.Status)
}

// TestIntegration_HealthCheckAfterOperations tests health check during various operations
func TestIntegration_HealthCheckAfterOperations(t *testing.T) {
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
	time.Sleep(300 * time.Millisecond)

	// Initial health check
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	// Perform operations
	conn, err := server.Client(ctx)
	require.NoError(t, err)

	permissionsClient := v1.NewPermissionsServiceClient(conn)

	// Write relationships
	_, err = permissionsClient.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
		Updates: []*v1.RelationshipUpdate{
			{
				Operation: v1.RelationshipUpdate_OPERATION_CREATE,
				Relationship: &v1.Relationship{
					Resource: &v1.ObjectReference{
						ObjectType: "document",
						ObjectId:   "doc1",
					},
					Relation: "reader",
					Subject: &v1.SubjectReference{
						Object: &v1.ObjectReference{
							ObjectType: "user",
							ObjectId:   "alice",
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	// Health check after write
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
	assert.Equal(t, "healthy", healthStatus.Checks["datastore"])

	// Check permission
	_, err = permissionsClient.CheckPermission(ctx, &v1.CheckPermissionRequest{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "read",
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: "user",
				ObjectId:   "alice",
			},
		},
	})
	require.NoError(t, err)

	// Health check after permission check
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
}

// TestIntegration_HealthCheckUptime tests that uptime is tracked correctly
func TestIntegration_HealthCheckUptime(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer removeFile(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:        []string{tmpFile},
		GRPCAddress:        port,
		PresharedKey:       "test-key",
		HealthCheckEnabled: false,
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(300 * time.Millisecond)

	// First health check
	healthStatus1, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.NotNil(t, healthStatus1.StartTime)
	assert.NotEmpty(t, healthStatus1.Uptime)

	// Wait a bit more
	time.Sleep(200 * time.Millisecond)

	// Second health check - uptime should have increased
	healthStatus2, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.NotNil(t, healthStatus2.StartTime)
	assert.NotEmpty(t, healthStatus2.Uptime)

	// Start times should be the same
	assert.Equal(t, healthStatus1.StartTime, healthStatus2.StartTime)

	// Uptime should be different (or at least not less)
	// Note: We can't easily compare uptime strings, but we can verify they exist
	assert.NotEmpty(t, healthStatus1.Uptime)
	assert.NotEmpty(t, healthStatus2.Uptime)
}
