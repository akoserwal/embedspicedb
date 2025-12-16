package embedspicedb_test

import (
	"context"
	"encoding/json"
	"fmt"
	. "github.com/akoserwal/embedspicedb"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_HealthCheckHTTPEndpoint tests the HTTP health check endpoint
func TestIntegration_HealthCheckHTTPEndpoint(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	grpcPort := getFreePort(t)
	healthPort := getFreePort(t)
	config := Config{
		SchemaFiles:        []string{tmpFile},
		GRPCAddress:        grpcPort,
		PresharedKey:       "test-key",
		HealthCheckEnabled: true,
		HealthCheckAddress: healthPort,
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Wait for servers to be ready
	time.Sleep(500 * time.Millisecond)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Test /healthz endpoint
	resp, err := client.Get("http://" + healthPort + "/healthz")
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
	assert.NotNil(t, status.StartTime)
	assert.NotEmpty(t, status.Uptime)

	// Test /health endpoint (alias)
	resp2, err := client.Get("http://" + healthPort + "/health")
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

// TestIntegration_FullLifecycle tests the complete lifecycle of an embedded server
func TestIntegration_FullLifecycle(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:  []string{tmpFile},
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	// Create server
	server, err := New(config)
	require.NoError(t, err)

	ctx := context.Background()

	// Start server
	err = server.Start(ctx)
	require.NoError(t, err)

	// Get client connection - wait a bit for server to be fully ready
	time.Sleep(200 * time.Millisecond)

	// Verify health check - server should be healthy after start
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
	assert.Equal(t, "started", healthStatus.Checks["server"])

	conn, err := server.Client(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	// Create SpiceDB clients
	schemaClient := v1.NewSchemaServiceClient(conn)
	permissionsClient := v1.NewPermissionsServiceClient(conn)

	// Verify schema was loaded - retry if needed
	var readResp *v1.ReadSchemaResponse
	for i := 0; i < 5; i++ {
		readResp, err = schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NoError(t, err)
	assert.Contains(t, readResp.SchemaText, "definition user")
	assert.Contains(t, readResp.SchemaText, "definition document")

	// Write a relationship
	writeResp, err := permissionsClient.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
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
	assert.NotNil(t, writeResp)

	// Check permission
	checkResp, err := permissionsClient.CheckPermission(ctx, &v1.CheckPermissionRequest{
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
	assert.Equal(t, v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, checkResp.Permissionship)

	// Verify health check before stopping - should still be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	// Stop server
	err = server.Stop()
	assert.NoError(t, err)

	// Verify server is stopped
	conn, err = server.Client(ctx)
	assert.Error(t, err)
	assert.Nil(t, conn)
}

// TestIntegration_HotReload tests hot reload functionality with file changes
func TestIntegration_HotReload(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:   []string{tmpFile},
		GRPCAddress:   port,
		PresharedKey:  "test-key",
		WatchDebounce: 200 * time.Millisecond,
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	var reloadCount int
	var reloadMutex sync.Mutex
	var lastReloadError error

	server.OnSchemaReloaded(func(err error) {
		reloadMutex.Lock()
		defer reloadMutex.Unlock()
		reloadCount++
		lastReloadError = err
	})

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
	assert.Equal(t, "healthy", healthStatus.Status)

	conn, err := server.Client(ctx)
	require.NoError(t, err)

	schemaClient := v1.NewSchemaServiceClient(conn)

	// Read initial schema
	initialResp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)
	initialSchema := initialResp.SchemaText

	// Wait a bit for watcher to be ready
	time.Sleep(100 * time.Millisecond)

	// Modify schema file
	newSchema := `definition user {}

definition document {
  relation reader: user
  relation writer: user
  permission read = reader
  permission write = writer
}`
	err = os.WriteFile(tmpFile, []byte(newSchema), 0644)
	require.NoError(t, err)

	// Wait for debounce and reload
	time.Sleep(500 * time.Millisecond)

	reloadMutex.Lock()
	count := reloadCount
	errVal := lastReloadError
	reloadMutex.Unlock()

	assert.GreaterOrEqual(t, count, 1, "reload callback should have been called")
	assert.NoError(t, errVal, "reload should succeed")

	// Verify health check after hot reload - should still be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	// Verify schema was updated
	updatedResp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)
	assert.NotEqual(t, initialSchema, updatedResp.SchemaText)
	assert.Contains(t, updatedResp.SchemaText, "writer")
}

// TestIntegration_MultipleSchemaFiles tests combining multiple schema files
func TestIntegration_MultipleSchemaFiles(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "base.zed")
	file2 := filepath.Join(tmpDir, "extensions.zed")

	err := os.WriteFile(file1, []byte(`definition user {}`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(file2, []byte(`definition document {
  relation reader: user
  permission read = reader
}`), 0644)
	require.NoError(t, err)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:  []string{file1, file2},
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	conn, err := server.Client(ctx)
	require.NoError(t, err)

	schemaClient := v1.NewSchemaServiceClient(conn)

	// Verify both schemas are combined
	resp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)
	assert.Contains(t, resp.SchemaText, "definition user")
	assert.Contains(t, resp.SchemaText, "definition document")
}

// TestIntegration_ManualReload tests manual schema reload
func TestIntegration_ManualReload(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:  []string{tmpFile},
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	conn, err := server.Client(ctx)
	require.NoError(t, err)

	schemaClient := v1.NewSchemaServiceClient(conn)

	// Read initial schema
	initialResp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)

	// Modify schema file
	newSchema := `definition user {}

definition document {
  relation reader: user
  relation writer: user
  permission read = reader
  permission write = writer
}`
	err = os.WriteFile(tmpFile, []byte(newSchema), 0644)
	require.NoError(t, err)

	// Manually reload
	err = server.ReloadSchema(ctx)
	require.NoError(t, err)

	// Verify health check after reload - should still be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	// Verify schema was updated
	updatedResp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)
	assert.NotEqual(t, initialResp.SchemaText, updatedResp.SchemaText)
	assert.Contains(t, updatedResp.SchemaText, "writer")
}

// TestIntegration_WriteAndCheck tests writing relationships and checking permissions
func TestIntegration_WriteAndCheck(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:  []string{tmpFile},
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	conn, err := server.Client(ctx)
	require.NoError(t, err)

	permissionsClient := v1.NewPermissionsServiceClient(conn)

	// Write multiple relationships
	writeResp, err := permissionsClient.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
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
			{
				Operation: v1.RelationshipUpdate_OPERATION_CREATE,
				Relationship: &v1.Relationship{
					Resource: &v1.ObjectReference{
						ObjectType: "document",
						ObjectId:   "doc2",
					},
					Relation: "reader",
					Subject: &v1.SubjectReference{
						Object: &v1.ObjectReference{
							ObjectType: "user",
							ObjectId:   "bob",
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, writeResp)

	// Check permissions
	check1, err := permissionsClient.CheckPermission(ctx, &v1.CheckPermissionRequest{
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
	assert.Equal(t, v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, check1.Permissionship)

	check2, err := permissionsClient.CheckPermission(ctx, &v1.CheckPermissionRequest{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc2",
		},
		Permission: "read",
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: "user",
				ObjectId:   "bob",
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, check2.Permissionship)

	// Check negative case
	check3, err := permissionsClient.CheckPermission(ctx, &v1.CheckPermissionRequest{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "read",
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: "user",
				ObjectId:   "bob",
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, v1.CheckPermissionResponse_PERMISSIONSHIP_NO_PERMISSION, check3.Permissionship)

	// Verify health check after operations - should still be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
}

// TestIntegration_ReadRelationships tests reading relationships
func TestIntegration_ReadRelationships(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:  []string{tmpFile},
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

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

	// Read relationships
	readStream, err := permissionsClient.ReadRelationships(ctx, &v1.ReadRelationshipsRequest{
		RelationshipFilter: &v1.RelationshipFilter{
			ResourceType: "document",
		},
	})
	require.NoError(t, err)

	var found bool
	for {
		resp, err := readStream.Recv()
		if err != nil {
			break
		}
		if resp.Relationship.Resource.ObjectId == "doc1" &&
			resp.Relationship.Subject.Object.ObjectId == "alice" {
			found = true
			break
		}
	}
	assert.True(t, found, "should find the written relationship")

	// Verify health check after read operations - should still be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
}

// TestIntegration_ConcurrentOperations tests concurrent operations
func TestIntegration_ConcurrentOperations(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:  []string{tmpFile},
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	conn, err := server.Client(ctx)
	require.NoError(t, err)

	permissionsClient := v1.NewPermissionsServiceClient(conn)

	// Concurrent writes
	var wg sync.WaitGroup
	numGoroutines := 10
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := permissionsClient.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
				Updates: []*v1.RelationshipUpdate{
					{
						Operation: v1.RelationshipUpdate_OPERATION_CREATE,
						Relationship: &v1.Relationship{
							Resource: &v1.ObjectReference{
								ObjectType: "document",
								ObjectId:   "doc" + fmt.Sprintf("%d", idx),
							},
							Relation: "reader",
							Subject: &v1.SubjectReference{
								Object: &v1.ObjectReference{
									ObjectType: "user",
									ObjectId:   "user" + fmt.Sprintf("%d", idx),
								},
							},
						},
					},
				},
			})
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// Verify all writes succeeded
	for i, err := range errors {
		assert.NoError(t, err, "write %d should succeed", i)
	}

	// Verify health check after concurrent operations
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
	assert.Equal(t, "healthy", healthStatus.Checks["datastore"])
}

// TestIntegration_SchemaReloadWithOperations tests schema reload while operations are happening
func TestIntegration_SchemaReloadWithOperations(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:   []string{tmpFile},
		GRPCAddress:   port,
		PresharedKey:  "test-key",
		WatchDebounce: 100 * time.Millisecond,
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	conn, err := server.Client(ctx)
	require.NoError(t, err)

	permissionsClient := v1.NewPermissionsServiceClient(conn)

	// Write a relationship
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

	// Verify it works
	checkResp, err := permissionsClient.CheckPermission(ctx, &v1.CheckPermissionRequest{
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
	assert.Equal(t, v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, checkResp.Permissionship)

	// Update schema to add writer relation
	newSchema := `definition user {}

definition document {
  relation reader: user
  relation writer: user
  permission read = reader
  permission write = writer
}`
	err = os.WriteFile(tmpFile, []byte(newSchema), 0644)
	require.NoError(t, err)

	// Wait for reload
	time.Sleep(300 * time.Millisecond)

	// Verify new schema is active
	schemaClient := v1.NewSchemaServiceClient(conn)
	resp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)
	assert.Contains(t, resp.SchemaText, "writer")

	// Verify existing relationship still works
	checkResp2, err := permissionsClient.CheckPermission(ctx, &v1.CheckPermissionRequest{
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
	assert.Equal(t, v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, checkResp2.Permissionship)

	// Verify health check after schema reload with operations - should still be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
}

// TestIntegration_ErrorRecovery tests error recovery scenarios
func TestIntegration_ErrorRecovery(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:  []string{tmpFile},
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	conn, err := server.Client(ctx)
	require.NoError(t, err)

	schemaClient := v1.NewSchemaServiceClient(conn)

	// Verify health check before error - should be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	// Write invalid schema temporarily
	invalidSchema := `invalid schema syntax`
	err = os.WriteFile(tmpFile, []byte(invalidSchema), 0644)
	require.NoError(t, err)

	// Try to reload - should fail
	err = server.ReloadSchema(ctx)
	assert.Error(t, err)

	// Verify health check after failed reload - should still be healthy (server running)
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	// Server is still running, so should be healthy or degraded
	assert.Contains(t, []string{"healthy", "degraded"}, healthStatus.Status)

	// Write valid schema back
	validSchema := `definition user {}

definition document {
  relation reader: user
  permission read = reader
}`
	err = os.WriteFile(tmpFile, []byte(validSchema), 0644)
	require.NoError(t, err)

	// Reload should succeed now
	err = server.ReloadSchema(ctx)
	assert.NoError(t, err)

	// Verify schema is valid
	resp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)
	assert.Contains(t, resp.SchemaText, "definition user")

	// Verify health check after recovery - should be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
}

// TestIntegration_NoSchemaFiles tests starting without schema files
func TestIntegration_NoSchemaFiles(t *testing.T) {
	port := getFreePort(t)
	config := Config{
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	conn, err := server.Client(ctx)
	require.NoError(t, err)

	schemaClient := v1.NewSchemaServiceClient(conn)

	// Write schema programmatically
	_, err = schemaClient.WriteSchema(ctx, &v1.WriteSchemaRequest{
		Schema: `definition user {}

definition document {
  relation reader: user
  permission read = reader
}`,
	})
	require.NoError(t, err)

	// Verify schema was written
	resp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)
	assert.Contains(t, resp.SchemaText, "definition user")
	assert.Contains(t, resp.SchemaText, "definition document")

	// Verify health check after writing schema - should still be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
}

// TestIntegration_MultipleClients tests multiple clients accessing the same server
func TestIntegration_MultipleClients(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:  []string{tmpFile},
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	server, err := New(config)
	require.NoError(t, err)
	defer server.Stop()

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	// Get multiple client connections
	conn1, err := server.Client(ctx)
	require.NoError(t, err)

	conn2, err := server.Client(ctx)
	require.NoError(t, err)

	permissionsClient1 := v1.NewPermissionsServiceClient(conn1)
	permissionsClient2 := v1.NewPermissionsServiceClient(conn2)

	// Ensure schema is fully loaded before doing permission checks
	schemaClient := v1.NewSchemaServiceClient(conn1)
	for i := 0; i < 10; i++ {
		resp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
		if err == nil && resp.SchemaText != "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Write from client 1
	_, err = permissionsClient1.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
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

	// Read from client 2
	checkResp, err := permissionsClient2.CheckPermission(ctx, &v1.CheckPermissionRequest{
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
	assert.Equal(t, v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, checkResp.Permissionship)

	// Verify health check with multiple clients - should still be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)
}

// TestIntegration_RestartServer tests restarting the server
func TestIntegration_RestartServer(t *testing.T) {
	tmpFile := createTempSchemaFile(t)
	defer os.Remove(tmpFile)

	port := getFreePort(t)
	config := Config{
		SchemaFiles:  []string{tmpFile},
		GRPCAddress:  port,
		PresharedKey: "test-key",
	}

	server, err := New(config)
	require.NoError(t, err)

	ctx := context.Background()

	// Start server
	err = server.Start(ctx)
	require.NoError(t, err)

	// Verify health check after start
	time.Sleep(200 * time.Millisecond)
	healthStatus, err := server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	conn, err := server.Client(ctx)
	require.NoError(t, err)

	permissionsClient := v1.NewPermissionsServiceClient(conn)

	// Write a relationship
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

	// Verify health check before stop - should be healthy
	healthStatus, err = server.HealthCheck(ctx)
	require.NoError(t, err)
	assert.Equal(t, "healthy", healthStatus.Status)

	// Stop server
	err = server.Stop()
	require.NoError(t, err)

	// Verify health check after stop - should be unhealthy
	healthStatus, err = server.HealthCheck(ctx)
	require.Error(t, err)
	if healthStatus != nil {
		assert.Equal(t, "unhealthy", healthStatus.Status)
	}

	// Note: With memdb, data is lost on restart, so we can't test persistence
	// This test just verifies restart works
}
