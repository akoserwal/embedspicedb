package schema

import (
	"context"
	"fmt"
	"io"
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
	var yamlFiles []string

	for _, filePath := range r.files {
		ext := strings.ToLower(filepath.Ext(filePath))
		if ext == ".yaml" || ext == ".yml" {
			yamlFiles = append(yamlFiles, filePath)
		} else {
			// Assume .zed or plain text schema
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read schema file %s: %w", filePath, err)
			}
			schemaParts = append(schemaParts, string(content))
		}
	}

	// Handle YAML validation files
	if len(yamlFiles) > 0 {
		for _, yamlFile := range yamlFiles {
			content, err := os.ReadFile(yamlFile)
			if err != nil {
				return fmt.Errorf("failed to read YAML file %s: %w", yamlFile, err)
			}

			parsed, err := validationfile.DecodeValidationFile(content)
			if err != nil {
				// Fallback: allow minimal YAML with schema/schema_file keys.
				var m map[string]any
				if yerr := yaml.Unmarshal(content, &m); yerr != nil {
					return fmt.Errorf("failed to parse YAML file %s: %w", yamlFile, err)
				}
				if v, ok := m["schema"]; ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						schemaParts = append(schemaParts, s)
						continue
					}
				}
				if v, ok := m["schema_file"]; ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						ref := s
						if !filepath.IsLocal(ref) {
							return fmt.Errorf("schema file %q is not local", ref)
						}
						schemaPath := filepath.Join(filepath.Dir(yamlFile), ref)
						schemaContent, err := os.ReadFile(schemaPath)
						if err != nil {
							return fmt.Errorf("failed to read referenced schema file %s: %w", schemaPath, err)
						}
						schemaParts = append(schemaParts, string(schemaContent))
						continue
					}
				}

				return fmt.Errorf("failed to parse YAML file %s: %w", yamlFile, err)
			}

			if parsed.Schema.Schema != "" {
				schemaParts = append(schemaParts, parsed.Schema.Schema)
			} else if parsed.SchemaFile != "" {
				ref := parsed.SchemaFile
				if !filepath.IsLocal(ref) {
					return fmt.Errorf("schema file %q is not local", ref)
				}
				schemaPath := filepath.Join(filepath.Dir(yamlFile), ref)
				schemaContent, err := os.ReadFile(schemaPath)
				if err != nil {
					return fmt.Errorf("failed to read referenced schema file %s: %w", schemaPath, err)
				}
				schemaParts = append(schemaParts, string(schemaContent))
			} else {
				// Fallback for minimal YAML where schema_file isn't part of validationfile fields.
				var m map[string]any
				if err := yaml.Unmarshal(content, &m); err == nil {
					if v, ok := m["schema_file"]; ok {
						if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
							ref := s
							if !filepath.IsLocal(ref) {
								return fmt.Errorf("schema file %q is not local", ref)
							}
							schemaPath := filepath.Join(filepath.Dir(yamlFile), ref)
							schemaContent, err := os.ReadFile(schemaPath)
							if err != nil {
								return fmt.Errorf("failed to read referenced schema file %s: %w", schemaPath, err)
							}
							schemaParts = append(schemaParts, string(schemaContent))
						}
					}
				}
			}
		}
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

		parsed, err := validationfile.DecodeValidationFile(content)
		if err != nil {
			// Fallback: allow minimal YAML files that only contain schema/schema_file keys.
			var m map[string]any
			if yerr := yaml.Unmarshal(content, &m); yerr != nil {
				return "", fmt.Errorf("failed to parse YAML file: %w", err)
			}
			if v, ok := m["schema"]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					return s, nil
				}
			}
			if v, ok := m["schema_file"]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					ref := s
					if !filepath.IsLocal(ref) {
						return "", fmt.Errorf("schema file %q is not local", ref)
					}
					schemaPath := filepath.Join(filepath.Dir(filePath), ref)
					schemaContent, err := os.ReadFile(schemaPath)
					if err != nil {
						return "", fmt.Errorf("failed to read referenced schema file: %w", err)
					}
					return string(schemaContent), nil
				}
			}
			if v, ok := m["schemaFile"]; ok { // alternate spelling
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					ref := s
					if !filepath.IsLocal(ref) {
						return "", fmt.Errorf("schema file %q is not local", ref)
					}
					schemaPath := filepath.Join(filepath.Dir(filePath), ref)
					schemaContent, err := os.ReadFile(schemaPath)
					if err != nil {
						return "", fmt.Errorf("failed to read referenced schema file: %w", err)
					}
					return string(schemaContent), nil
				}
			}

			return "", fmt.Errorf("failed to parse YAML file: %w", err)
		}

		if parsed.Schema.Schema != "" {
			return parsed.Schema.Schema, nil
		} else if parsed.SchemaFile != "" {
			ref := parsed.SchemaFile
			if !filepath.IsLocal(ref) {
				return "", fmt.Errorf("schema file %q is not local", ref)
			}
			schemaPath := filepath.Join(filepath.Dir(filePath), ref)
			schemaContent, err := os.ReadFile(schemaPath)
			if err != nil {
				return "", fmt.Errorf("failed to read referenced schema file: %w", err)
			}
			return string(schemaContent), nil
		}
		// Fallback for minimal YAML: schema_file can exist outside validationfile's schema.
		var m map[string]any
		if err := yaml.Unmarshal(content, &m); err == nil {
			if v, ok := m["schema_file"]; ok {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					ref := s
					if !filepath.IsLocal(ref) {
						return "", fmt.Errorf("schema file %q is not local", ref)
					}
					schemaPath := filepath.Join(filepath.Dir(filePath), ref)
					schemaContent, err := os.ReadFile(schemaPath)
					if err != nil {
						return "", fmt.Errorf("failed to read referenced schema file: %w", err)
					}
					return string(schemaContent), nil
				}
			}
		}
		return "", fmt.Errorf("no schema found in YAML file")
	}

	// Read as plain text (.zed or other)
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	return string(content), nil
}
