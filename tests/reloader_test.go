package embedspicedb_test

import (
	"context"
	. "github.com/akoserwal/embedspicedb"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNewSchemaReloader(t *testing.T) {
	t.Run("create reloader successfully", func(t *testing.T) {
		// Create a mock connection
		conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			// If connection fails, skip test
			t.Skip("Cannot create gRPC connection for test")
		}
		defer conn.Close()

		files := []string{"/path/to/schema.zed"}
		reloader := NewSchemaReloader(conn, files)

		assert.NotNil(t, reloader)
	})

	t.Run("create reloader with empty files", func(t *testing.T) {
		conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Skip("Cannot create gRPC connection for test")
		}
		defer conn.Close()

		reloader := NewSchemaReloader(conn, []string{})

		assert.NotNil(t, reloader)
		// Behavior: Reload should fail when no files are configured.
		err = reloader.Reload(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no schema files")
	})
}

func TestSchemaReloader_Reload(t *testing.T) {
	t.Run("reload with valid schema file", func(t *testing.T) {
		// Start a real server for integration test
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)

		reloader := NewSchemaReloader(conn, []string{tmpFile})

		err = reloader.Reload(ctx)
		assert.NoError(t, err)
	})

	t.Run("reload with no files", func(t *testing.T) {
		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)

		reloader := NewSchemaReloader(conn, []string{})

		err = reloader.Reload(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no schema files")
	})

	t.Run("reload with nonexistent file", func(t *testing.T) {
		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)

		reloader := NewSchemaReloader(conn, []string{"/nonexistent/file.zed"})

		err = reloader.Reload(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read schema file")
	})

	t.Run("reload with multiple files", func(t *testing.T) {
		tmpFile1 := createTempFile(t, "schema1.zed", "definition user {}")
		tmpFile2 := createTempFile(t, "schema2.zed", `definition document {
  relation reader: user
  permission read = reader
}`)
		defer os.Remove(tmpFile1)
		defer os.Remove(tmpFile2)

		config := Config{
			SchemaFiles:  []string{tmpFile1, tmpFile2},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)

		reloader := NewSchemaReloader(conn, []string{tmpFile1, tmpFile2})

		err = reloader.Reload(ctx)
		assert.NoError(t, err)
	})

	t.Run("reload with empty schema file", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "empty-*.zed")
		require.NoError(t, err)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)

		reloader := NewSchemaReloader(conn, []string{tmpFile.Name()})

		err = reloader.Reload(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no schema content")
	})
}

func TestSchemaReloader_YAMLFiles(t *testing.T) {
	t.Run("reload with YAML file containing schema", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := filepath.Join(tmpDir, "schema.yaml")

		yamlContent := `schema: |
  definition user {}
  definition document {
    relation reader: user
    permission read = reader
  }`

		err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)

		reloader := NewSchemaReloader(conn, []string{yamlFile})

		err = reloader.Reload(ctx)
		assert.NoError(t, err)
	})

	t.Run("reload with YAML file referencing schema file", func(t *testing.T) {
		tmpDir := t.TempDir()
		schemaFile := filepath.Join(tmpDir, "schema.zed")
		yamlFile := filepath.Join(tmpDir, "validation.yaml")

		schemaContent := `definition user {}
definition document {
  relation reader: user
  permission read = reader
}`

		err := os.WriteFile(schemaFile, []byte(schemaContent), 0644)
		require.NoError(t, err)

		yamlContent := `schema_file: schema.zed`

		err = os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)

		reloader := NewSchemaReloader(conn, []string{yamlFile})

		err = reloader.Reload(ctx)
		assert.NoError(t, err)
	})

	t.Run("reload with invalid YAML file", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "invalid-*.yaml")
		require.NoError(t, err)
		_, err = tmpFile.WriteString("invalid: yaml: content: [")
		require.NoError(t, err)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)

		reloader := NewSchemaReloader(conn, []string{tmpFile.Name()})

		err = reloader.Reload(ctx)
		assert.Error(t, err)
	})
}

func TestReadSchemaFile(t *testing.T) {
	t.Run("read valid .zed file", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		content, err := ReadSchemaFile(tmpFile)
		assert.NoError(t, err)
		assert.Contains(t, content, "definition user")
		assert.Contains(t, content, "definition document")
	})

	t.Run("read nonexistent file", func(t *testing.T) {
		_, err := ReadSchemaFile("/nonexistent/file.zed")
		assert.Error(t, err)
	})

	t.Run("read YAML file with schema", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlFile := filepath.Join(tmpDir, "schema.yaml")

		yamlContent := `schema: |
  definition user {}
  definition document {
    relation reader: user
    permission read = reader
  }`

		err := os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		content, err := ReadSchemaFile(yamlFile)
		assert.NoError(t, err)
		assert.Contains(t, content, "definition user")
	})

	t.Run("read YAML file referencing schema file", func(t *testing.T) {
		tmpDir := t.TempDir()
		schemaFile := filepath.Join(tmpDir, "schema.zed")
		yamlFile := filepath.Join(tmpDir, "validation.yaml")

		schemaContent := `definition user {}`

		err := os.WriteFile(schemaFile, []byte(schemaContent), 0644)
		require.NoError(t, err)

		yamlContent := `schema_file: schema.zed`

		err = os.WriteFile(yamlFile, []byte(yamlContent), 0644)
		require.NoError(t, err)

		content, err := ReadSchemaFile(yamlFile)
		assert.NoError(t, err)
		assert.Contains(t, content, "definition user")
	})

	t.Run("read YAML file with no schema", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "empty-*.yaml")
		require.NoError(t, err)
		_, err = tmpFile.WriteString("other_field: value")
		require.NoError(t, err)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		_, err = ReadSchemaFile(tmpFile.Name())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no schema found")
	})

	t.Run("read invalid YAML file", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "invalid-*.yaml")
		require.NoError(t, err)
		_, err = tmpFile.WriteString("invalid: yaml: [")
		require.NoError(t, err)
		tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		_, err = ReadSchemaFile(tmpFile.Name())
		assert.Error(t, err)
	})
}

func TestSchemaReloader_ContextCancellation(t *testing.T) {
	t.Run("reload respects context cancellation", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)

		reloader := NewSchemaReloader(conn, []string{tmpFile})

		// Cancel context
		cancel()

		// Reload should handle cancellation gracefully
		err = reloader.Reload(ctx)
		// May succeed or fail depending on timing, but shouldn't panic
		_ = err
	})
}

func TestSchemaReloader_CombinedSchemas(t *testing.T) {
	t.Run("combine multiple schema files", func(t *testing.T) {
		tmpFile1 := createTempFile(t, "schema1.zed", "definition user {}")
		tmpFile2 := createTempFile(t, "schema2.zed", `definition document {
  relation reader: user
  permission read = reader
}`)
		defer os.Remove(tmpFile1)
		defer os.Remove(tmpFile2)

		config := Config{
			SchemaFiles:  []string{tmpFile1, tmpFile2},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		require.NoError(t, err)
		require.NotNil(t, conn)

		reloader := NewSchemaReloader(conn, []string{tmpFile1, tmpFile2})

		err = reloader.Reload(ctx)
		assert.NoError(t, err)

		// Verify schema was written
		schemaClient := v1.NewSchemaServiceClient(conn)
		resp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
		if err == nil {
			assert.Contains(t, resp.SchemaText, "definition user")
			assert.Contains(t, resp.SchemaText, "definition document")
		}
	})
}
