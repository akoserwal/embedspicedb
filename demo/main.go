package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/akoserwal/embedspicedb"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func main() {
	fmt.Println("🚀 SpiceDB Embedded Server Demo")
	fmt.Println("===============================")
	fmt.Println()

	// Create a temporary directory for demo files
	tmpDir, err := os.MkdirTemp("", "spicedb-demo-*")
	if err != nil {
		log.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	schemaFile := filepath.Join(tmpDir, "schema.zed")
	writeSchemaFile(schemaFile)

	fmt.Printf("📝 Created schema file: %s\n\n", schemaFile)

	// Configure embedded server
	config := embedspicedb.Config{
		SchemaFiles:   []string{schemaFile},
		GRPCAddress:   ":50051",
		PresharedKey:  "demo-key",
		WatchDebounce: 500 * time.Millisecond,
	}

	fmt.Println("⚙️  Configuration:")
	fmt.Printf("   - Schema Files: %v\n", config.SchemaFiles)
	fmt.Printf("   - gRPC Address: %s\n", config.GRPCAddress)
	fmt.Printf("   - Preshared Key: %s\n\n", config.PresharedKey)

	// Create server
	server, err := embedspicedb.New(config)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Register callback for schema reload events
	server.OnSchemaReloaded(func(err error) {
		if err != nil {
			fmt.Printf("⚠️  Schema reload error: %v\n", err)
		} else {
			fmt.Println("✅ Schema reloaded successfully")
		}
	})

	// Start server
	ctx := context.Background()
	fmt.Println("🔄 Starting embedded SpiceDB server...")
	if err := server.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()
	fmt.Println("✅ Server started successfully!")
	fmt.Println()

	// Get client connection
	conn, err := server.Client(ctx)
	if err != nil {
		log.Fatalf("Failed to get client: %v", err)
	}

	// Create clients
	schemaClient := v1.NewSchemaServiceClient(conn)
	permissionsClient := v1.NewPermissionsServiceClient(conn)

	// Demo 1: Read schema
	fmt.Println("📖 Demo 1: Reading Schema")
	fmt.Println("---------------------------")
	readResp, err := schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	if err != nil {
		log.Fatalf("Failed to read schema: %v", err)
	}
	fmt.Printf("Schema loaded: %d characters\n", len(readResp.SchemaText))
	fmt.Printf("Schema content:\n%s\n\n", readResp.SchemaText)

	// Demo 2: Write relationships
	fmt.Println("✍️  Demo 2: Writing Relationships")
	fmt.Println("-----------------------------------")

	// Write: alice is a reader of document1
	fmt.Println("Writing: alice is a reader of document1")
	_, err = permissionsClient.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
		Updates: []*v1.RelationshipUpdate{
			{
				Operation: v1.RelationshipUpdate_OPERATION_CREATE,
				Relationship: &v1.Relationship{
					Resource: &v1.ObjectReference{
						ObjectType: "document",
						ObjectId:   "document1",
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
	if err != nil {
		log.Fatalf("Failed to write relationship: %v", err)
	}
	fmt.Println("✅ Relationship written successfully")

	// Write: bob is a writer of document1
	fmt.Println("Writing: bob is a writer of document1")
	_, err = permissionsClient.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
		Updates: []*v1.RelationshipUpdate{
			{
				Operation: v1.RelationshipUpdate_OPERATION_CREATE,
				Relationship: &v1.Relationship{
					Resource: &v1.ObjectReference{
						ObjectType: "document",
						ObjectId:   "document1",
					},
					Relation: "writer",
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
	if err != nil {
		log.Fatalf("Failed to write relationship: %v", err)
	}
	fmt.Println("✅ Relationship written successfully")
	fmt.Println()

	// Demo 3: Check permissions
	fmt.Println("🔍 Demo 3: Checking Permissions")
	fmt.Println("-------------------------------")

	checkPermission := func(user, document, permission string) {
		resp, err := permissionsClient.CheckPermission(ctx, &v1.CheckPermissionRequest{
			Resource: &v1.ObjectReference{
				ObjectType: "document",
				ObjectId:   document,
			},
			Permission: permission,
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   user,
				},
			},
		})
		if err != nil {
			log.Printf("Failed to check permission: %v", err)
			return
		}
		status := "❌ DENIED"
		if resp.Permissionship == v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION {
			status = "✅ ALLOWED"
		}
		fmt.Printf("   %s: %s can %s %s\n", status, user, permission, document)
	}

	checkPermission("alice", "document1", "read")
	checkPermission("alice", "document1", "write")
	checkPermission("bob", "document1", "read")
	checkPermission("bob", "document1", "write")
	checkPermission("charlie", "document1", "read")
	fmt.Println()

	// Demo 4: Hot reload
	fmt.Println("🔄 Demo 4: Hot Reload (Schema Change)")
	fmt.Println("--------------------------------------")

	// Update schema to add a new permission
	newSchema := `definition user {}

definition document {
	relation reader: user
	relation writer: user
	relation owner: user
	permission read = reader + owner
	permission write = writer + owner
	permission admin = owner
}`
	err = os.WriteFile(schemaFile, []byte(newSchema), 0644)
	if err != nil {
		log.Fatalf("Failed to write new schema: %v", err)
	}
	fmt.Println("📝 Updated schema file (added 'owner' relation and 'admin' permission)")
	fmt.Println("⏳ Waiting for hot reload (debounce: 500ms)...")
	time.Sleep(1 * time.Second) // Wait for file watcher to detect change

	// Write: alice is owner of document2
	fmt.Println()
	fmt.Println("Writing: alice is owner of document2")
	_, err = permissionsClient.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
		Updates: []*v1.RelationshipUpdate{
			{
				Operation: v1.RelationshipUpdate_OPERATION_CREATE,
				Relationship: &v1.Relationship{
					Resource: &v1.ObjectReference{
						ObjectType: "document",
						ObjectId:   "document2",
					},
					Relation: "owner",
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
	if err != nil {
		log.Fatalf("Failed to write relationship: %v", err)
	}

	// Check new permissions
	fmt.Println()
	fmt.Println("Checking permissions with new schema:")
	checkPermission("alice", "document2", "read")
	checkPermission("alice", "document2", "write")
	checkPermission("alice", "document2", "admin")
	fmt.Println()

	// Demo 5: Read relationships
	fmt.Println("📋 Demo 5: Reading Relationships")
	fmt.Println("-------------------------------")
	stream, err := permissionsClient.ReadRelationships(ctx, &v1.ReadRelationshipsRequest{
		RelationshipFilter: &v1.RelationshipFilter{
			ResourceType: "document",
		},
	})
	if err != nil {
		log.Fatalf("Failed to read relationships: %v", err)
	}

	fmt.Println("All relationships:")
	count := 0
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error reading relationships: %v", err)
			break
		}
		if resp.Relationship != nil {
			rel := resp.Relationship
			fmt.Printf("   - %s#%s@%s#%s\n",
				rel.Resource.ObjectType,
				rel.Resource.ObjectId,
				rel.Relation,
				rel.Subject.Object.ObjectId,
			)
			count++
		}
		if count >= 10 { // Limit output
			break
		}
	}
	if count == 0 {
		fmt.Println("   (no relationships found)")
	}
	fmt.Println()

	fmt.Println("✨ Demo completed successfully!")
	fmt.Println()
	fmt.Println("💡 Note: The server is still running. In a real application,")
	fmt.Println("   you would integrate this into your application lifecycle.")
	fmt.Println("   For this demo, the server will stop when the program exits.")
}

func writeSchemaFile(path string) {
	schema := `definition user {}

definition document {
	relation reader: user
	relation writer: user
	permission read = reader
	permission write = writer
}`
	if err := os.WriteFile(path, []byte(schema), 0644); err != nil {
		log.Fatalf("Failed to write schema file: %v", err)
	}
}
