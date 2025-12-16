package embedspicedb_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Helper function to create a temporary schema file
func createTempSchemaFile(t *testing.T) string {
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, "schema.zed")
	schema := `definition user {}

definition document {
  relation reader: user
  permission read = reader
}`
	require.NoError(t, os.WriteFile(tmpPath, []byte(schema), 0o644))
	return tmpPath
}

// Helper function to create a temporary file
func createTempFile(t *testing.T, name, content string) string {
	tmpDir := t.TempDir()
	tmpPath := filepath.Join(tmpDir, name)
	require.NoError(t, os.WriteFile(tmpPath, []byte(content), 0o644))
	return tmpPath
}

// getFreePort finds a free port for testing
func getFreePort(t *testing.T) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.String()
}
