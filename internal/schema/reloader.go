package schema

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/validationfile"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	log "github.com/akoserwal/embedspicedb/internal/logging"
)

// SchemaReloader handles reloading schema files into SpiceDB.
// NOTE: This lives in an internal package to keep the public surface area small.
type SchemaReloader struct {
	schemaClient v1.SchemaServiceClient
	files        []string
}

// NewSchemaReloader creates a new schema reloader.
func NewSchemaReloader(conn *grpc.ClientConn, schemaFiles []string) *SchemaReloader {
	return &SchemaReloader{
		schemaClient: v1.NewSchemaServiceClient(conn),
		files:        schemaFiles,
	}
}

// Reload reads and reloads all schema files.
func (r *SchemaReloader) Reload(ctx context.Context) error {
	if len(r.files) == 0 {
		return fmt.Errorf("no schema files configured")
	}

	// Read all schema files and combine them
	var schemaParts []string

	for _, filePath := range r.files {
		content, err := ReadSchemaFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read schema file %s: %w", filePath, err)
		}
		schemaParts = append(schemaParts, content)
	}

	combinedSchema := strings.Join(schemaParts, "\n\n")
	if combinedSchema == "" {
		return fmt.Errorf("no schema content found in files")
	}

	log.Ctx(ctx).Info().Int("files", len(r.files)).Msg("reloading schema")
	_, err := r.schemaClient.WriteSchema(ctx, &v1.WriteSchemaRequest{Schema: combinedSchema})
	if err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	log.Ctx(ctx).Info().Msg("schema reloaded successfully")
	return nil
}

// ReadSchemaFile reads a single schema file, handling both .zed and .yaml formats.
func ReadSchemaFile(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	if ext == ".yaml" || ext == ".yml" {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", err
		}

		schema, err := schemaFromYAML(filePath, content)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(schema) == "" {
			return "", fmt.Errorf("no schema found in YAML file")
		}
		return schema, nil
	}

	// Read as plain text (.zed or other)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func schemaFromYAML(yamlFilePath string, content []byte) (string, error) {
	parsed, err := validationfile.DecodeValidationFile(content)
	if err == nil {
		if parsed.Schema.Schema != "" {
			return parsed.Schema.Schema, nil
		}
		if parsed.SchemaFile != "" {
			return readReferencedSchemaFile(yamlFilePath, parsed.SchemaFile)
		}
		// Fall through to minimal YAML parsing below for cases not covered by validationfile.
	}

	var m map[string]any
	if yerr := yaml.Unmarshal(content, &m); yerr != nil {
		// Prefer the DecodeValidationFile error if we had one, but at least surface something consistent.
		if err != nil {
			return "", fmt.Errorf("failed to parse YAML file: %w", err)
		}
		return "", fmt.Errorf("failed to parse YAML file: %w", yerr)
	}

	if v, ok := m["schema"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s, nil
		}
	}
	if v, ok := m["schema_file"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return readReferencedSchemaFile(yamlFilePath, s)
		}
	}
	if v, ok := m["schemaFile"]; ok { // alternate spelling
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return readReferencedSchemaFile(yamlFilePath, s)
		}
	}

	if err != nil {
		return "", fmt.Errorf("failed to parse YAML file: %w", err)
	}
	return "", fmt.Errorf("no schema found in YAML file")
}

func readReferencedSchemaFile(yamlFilePath, ref string) (string, error) {
	if !filepath.IsLocal(ref) {
		return "", fmt.Errorf("schema file %q is not local", ref)
	}
	schemaPath := filepath.Join(filepath.Dir(yamlFilePath), ref)
	schemaContent, err := os.ReadFile(schemaPath)
	if err != nil {
		return "", fmt.Errorf("failed to read referenced schema file %s: %w", schemaPath, err)
	}
	return string(schemaContent), nil
}
