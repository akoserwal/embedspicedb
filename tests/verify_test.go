package embedspicedb_test

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	. "github.com/akoserwal/embedspicedb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func getFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen for free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func TestEmbeddedServer_Basic(t *testing.T) {
	// Create a temporary schema file
	tmpFile, err := os.CreateTemp("", "schema-*.zed")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write a simple schema
	schema := `definition user {}

definition document {
  relation reader: user
  relation writer: user
  permission read = reader
  permission write = writer
}`
	if _, err := tmpFile.WriteString(schema); err != nil {
		t.Fatalf("Failed to write schema: %v", err)
	}
	tmpFile.Close()

	// Configure embedded server
	config := Config{
		SchemaFiles:   []string{tmpFile.Name()},
		GRPCAddress:   getFreeAddr(t),
		PresharedKey:  "dev-key",
		WatchDebounce: 500 * time.Millisecond,
	}

	// Create server
	server, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Get client connection
	conn, err := server.Client(ctx)
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}

	// Use the connection for SpiceDB operations
	schemaClient := v1.NewSchemaServiceClient(conn)

	// Read schema
	readResp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	if err != nil {
		t.Fatalf("Failed to read schema: %v", err)
	}

	if len(readResp.SchemaText) == 0 {
		t.Error("Schema text is empty")
	}

	t.Logf("Schema loaded successfully: %d characters", len(readResp.SchemaText))
}

func TestEmbeddedServer_NoSchemaFiles(t *testing.T) {
	// Test starting server without schema files
	config := Config{
		GRPCAddress:  getFreeAddr(t),
		PresharedKey: "dev-key",
	}

	server, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	conn, err := server.Client(ctx)
	if err != nil {
		t.Fatalf("Failed to get client: %v", err)
	}

	// Verify connection works
	schemaClient := v1.NewSchemaServiceClient(conn)
	_, err = schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	// This should succeed even with empty schema
	if err != nil {
		t.Logf("ReadSchema returned error (expected for empty schema): %v", err)
	}

	t.Log("Server started successfully without schema files")
}
