package embedspicedb_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/akoserwal/embedspicedb"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func freeAddr() string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func ExampleEmbeddedServer_basic() {
	// Create a temporary schema file for demonstration
	tmpFile, err := os.CreateTemp("", "schema-*.zed")
	if err != nil {
		panic(err)
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
		panic(err)
	}
	tmpFile.Close()

	// Configure embedded server
	config := embedspicedb.Config{
		SchemaFiles:        []string{tmpFile.Name()},
		GRPCAddress:        freeAddr(),
		PresharedKey:       "dev-key",
		WatchDebounce:      500 * time.Millisecond,
		HealthCheckEnabled: false,
	}

	// Create server
	server, err := embedspicedb.New(config)
	if err != nil {
		panic(err)
	}

	// Register callback for schema reload events
	server.OnSchemaReloaded(func(err error) {
		_ = err
	})

	// Start server
	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		panic(err)
	}
	defer server.Stop()

	// Get client connection
	conn, err := server.Client(ctx)
	if err != nil {
		panic(err)
	}

	// Use the connection for SpiceDB operations
	schemaClient := v1.NewSchemaServiceClient(conn)

	// Read schema
	readResp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Schema loaded: %t\n", len(readResp.SchemaText) > 0)
	// Output: Schema loaded: true
}

func ExampleEmbeddedServer_customConfig() {
	// Example with custom configuration
	config := embedspicedb.DefaultConfig()
	config.GRPCAddress = freeAddr()
	config.PresharedKey = "my-custom-key"
	config.HealthCheckEnabled = false

	_, err := embedspicedb.New(config)
	fmt.Println(err == nil)
	// Output: true
}

func ExampleEmbeddedServer_manualReload() {
	config := embedspicedb.Config{
		SchemaFiles:        []string{},
		GRPCAddress:        freeAddr(),
		PresharedKey:       "dev-key",
		HealthCheckEnabled: false,
	}

	server, err := embedspicedb.New(config)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		panic(err)
	}
	defer server.Stop()

	// Manually reload schema
	err = server.ReloadSchema(ctx)
	fmt.Println(err != nil)
	// Output: true
}

func ExampleEmbeddedServer_noSchemaFiles() {
	// Start server without schema files (useful if you'll write schema programmatically)
	config := embedspicedb.Config{
		GRPCAddress:        freeAddr(),
		PresharedKey:       "dev-key",
		HealthCheckEnabled: false,
	}

	server, err := embedspicedb.New(config)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		panic(err)
	}
	defer server.Stop()

	conn, _ := server.Client(ctx)
	schemaClient := v1.NewSchemaServiceClient(conn)

	// Write schema programmatically
	_, err = schemaClient.WriteSchema(ctx, &v1.WriteSchemaRequest{
		Schema: `definition user {}`,
	})
	if err != nil {
		panic(err)
	}

	fmt.Println("Schema written programmatically")
	// Output: Schema written programmatically
}
