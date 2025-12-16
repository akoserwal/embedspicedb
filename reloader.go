package embedspicedb

import (
	"google.golang.org/grpc"

	internalschema "github.com/akoserwal/embedspicedb/internal/schema"
)

// SchemaReloader is a thin re-export of the internal schema reloader implementation.
// It is kept in the root package for backwards compatibility, while the implementation lives in `internal/schema`.
type SchemaReloader = internalschema.SchemaReloader

// NewSchemaReloader creates a new schema reloader.
func NewSchemaReloader(conn *grpc.ClientConn, schemaFiles []string) *SchemaReloader {
	return internalschema.NewSchemaReloader(conn, schemaFiles)
}

// ReadSchemaFile reads a single schema file, handling both .zed and .yaml formats.
func ReadSchemaFile(filePath string) (string, error) {
	return internalschema.ReadSchemaFile(filePath)
}
