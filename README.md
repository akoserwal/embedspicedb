# embedspicedb

A library for embedding SpiceDB with in-memory datastore and hot reload of schema files, optimized for development workflows.

## Installation

```bash
go get github.com/akoserwal/embedspicedb
```

**Note:** This library is now standalone and does not require SpiceDB source code for basic usage with the in-memory datastore.

## Features

- **Simple API**: Easy-to-use configuration and lifecycle management
- **Flexible Datastore**: Supports in-memory (memdb) or persistent (PostgreSQL, MySQL) datastores
- **Hot Reload**: Automatically watches schema files and reloads on changes
- **Development-Focused**: Optimized defaults for local development
- **Production-Ready**: Can use persistent datastores for production workloads

## Quick Start

### Basic Example

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/akoserwal/embedspicedb"
)

func main() {
    // Configure embedded server
    config := embedspicedb.Config{
        SchemaFiles:   []string{"./schema.zed"},
        GRPCAddress:   ":50051",
        PresharedKey:  "dev-key",
        WatchDebounce: 500 * time.Millisecond,
    }

    // Create server
    server, err := embedspicedb.New(config)
    if err != nil {
        log.Fatal(err)
    }

    // Register callback for schema reload events
    server.OnSchemaReloaded(func(err error) {
        if err != nil {
            log.Printf("Schema reload error: %v", err)
        } else {
            log.Println("Schema reloaded successfully")
        }
    })

    // Start server
    ctx := context.Background()
    if err := server.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer server.Stop()

    // Get client connection
    conn, err := server.Client(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // Use conn for SpiceDB operations...
}
```

### Complete Working Example with SpiceDB Client

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
    "github.com/authzed/authzed-go/v1"
    "github.com/akoserwal/embedspicedb"
)

func main() {
    // 1. Configure and start embedded server
    config := embedspicedb.Config{
        SchemaFiles:  []string{"./schema.zed"},
        GRPCAddress:  ":50051",
        PresharedKey: "dev-key",
    }

    server, err := embedspicedb.New(config)
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    if err := server.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer server.Stop()

    // 2. Get gRPC connection
    conn, err := server.Client(ctx)
    if err != nil {
        log.Fatal(err)
    }

    // 3. Create SpiceDB client
    client := v1.NewClient(conn, "dev-key")

    // 4. Use SpiceDB APIs
    // Write a relationship
    _, err = client.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
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
    if err != nil {
        log.Fatal(err)
    }

    // Check a permission
    checkResp, err := client.CheckPermission(ctx, &v1.CheckPermissionRequest{
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
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Permission check result: %v\n", checkResp.Permissionship == v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION)
}
```

## Configuration

### Default Configuration

```go
config := embedspicedb.DefaultConfig()
// Sets sensible defaults:
// - GRPCAddress: ":50051"
// - PresharedKey: "dev-key"
// - WatchDebounce: 500ms
// - RevisionQuantization: 5s
// - GCWindow: 24h
```

### Custom Configuration

```go
config := embedspicedb.Config{
    SchemaFiles:           []string{"./schema.zed", "./another.zed"},
    GRPCAddress:           ":50052",
    HTTPEnabled:           true,
    HTTPAddress:           ":8443",
    PresharedKey:          "my-key",
    WatchDebounce:         1 * time.Second,
    RevisionQuantization:  5 * time.Second,
    GCWindow:              24 * time.Hour,
    WatchBufferLength:     128,
}
```

### Persistent Datastore Configuration

**⚠️ Important:** Persistent datastore support (PostgreSQL/MySQL) is only available when using `embedspicedb` within the SpiceDB module context (requires SpiceDB source code access). In standalone mode, only `memdb` is available.

For production workloads requiring data persistence when using SpiceDB source:

```go
// PostgreSQL example (requires SpiceDB source access)
config := embedspicedb.Config{
    SchemaFiles:   []string{"./schema.zed"},
    GRPCAddress:   ":50051",
    PresharedKey:  "prod-key",
    DatastoreType: "postgres",
    DatastoreURI:  "postgres://user:password@localhost:5432/spicedb?sslmode=disable",
    RevisionQuantization: 5 * time.Second,
    GCWindow:      24 * time.Hour,
}

// MySQL example (requires SpiceDB source access)
config := embedspicedb.Config{
    SchemaFiles:   []string{"./schema.zed"},
    GRPCAddress:   ":50051",
    PresharedKey:  "prod-key",
    DatastoreType: "mysql",
    DatastoreURI:  "user:password@tcp(localhost:3306)/spicedb?parseTime=true",
    RevisionQuantization: 5 * time.Second,
    GCWindow:      24 * time.Hour,
}
```

**Supported Datastore Types:**
- `memdb` (default): In-memory datastore, non-persistent, perfect for development. **Available in standalone mode.**
- `postgres` or `postgresql`: PostgreSQL database (also compatible with CockroachDB). **Requires SpiceDB source access.**
- `mysql`: MySQL database. **Requires SpiceDB source access.**

**Note:** When using persistent datastores, ensure:
- The database is running and accessible
- The connection URI is correct
- Required database migrations will be run automatically by SpiceDB
- For MySQL, `parseTime=true` must be included in the connection string
- You have SpiceDB source code access and have set up the replace directive

## Schema Files

The library supports two types of schema files:

1. **Plain Schema Files** (`.zed` or any text file):
   ```zed
   definition user {}
   
   definition document {
     relation reader: user
     permission read: reader
   }
   ```

2. **YAML Validation Files** (`.yaml` or `.yml`):
   ```yaml
   schema: |
     definition user {}
     definition document {
       relation reader: user
       permission read: reader
     }
   ```

Multiple schema files are combined when reloaded.

## API Reference

### `New(config Config) (*EmbeddedServer, error)`

Creates a new embedded SpiceDB server with the given configuration.

### `Start(ctx context.Context) error`

Starts the server and begins watching schema files for changes. If schema files are configured, they are loaded immediately.

### `Stop() error`

Stops the server, file watchers, and cleans up resources.

### `Client(ctx context.Context) (*grpc.ClientConn, error)`

Returns a gRPC client connection to the embedded server.

### `ReloadSchema(ctx context.Context) error`

Manually reloads schema files. Useful for programmatic schema updates or testing.

### `OnSchemaReloaded(callback func(error))`

Registers a callback function that will be called whenever the schema is reloaded (either automatically via file watching or manually).

## Hot Reload

The library automatically watches schema files for changes. When a file is modified:

1. The change is detected by the file watcher
2. Changes are debounced (default: 500ms) to prevent rapid reloads
3. Schema files are read and combined
4. Schema is written to SpiceDB using the WriteSchema API
5. Registered callbacks are invoked with any errors

## Usage Guide

### Step-by-Step Setup

1. **Create a schema file** (`schema.zed`):
   ```zed
   definition user {}
   
   definition document {
     relation reader: user
     permission read = reader
   }
   ```

2. **Install the library**:
   ```bash
   go get github.com/akoserwal/embedspicedb
   go get github.com/authzed/authzed-go/v1
   ```

3. **Create and start the server**:
   ```go
   config := embedspicedb.Config{
       SchemaFiles:  []string{"./schema.zed"},
       GRPCAddress:  ":50051",
       PresharedKey: "dev-key",
   }
   
   server, err := embedspicedb.New(config)
   if err != nil {
       log.Fatal(err)
   }
   
   ctx := context.Background()
   if err := server.Start(ctx); err != nil {
       log.Fatal(err)
   }
   defer server.Stop()
   ```

4. **Get a client connection**:
   ```go
   conn, err := server.Client(ctx)
   if err != nil {
       log.Fatal(err)
   }
   ```

5. **Use SpiceDB APIs**:
   ```go
   client := v1.NewClient(conn, "dev-key")
   // Now you can use all SpiceDB APIs
   ```

### Common Use Cases

#### 1. Development/Testing Server

Perfect for local development and testing:

```go
config := embedspicedb.DefaultConfig()
config.SchemaFiles = []string{"./schema.zed"}
server, _ := embedspicedb.New(config)
server.Start(context.Background())
defer server.Stop()
```

#### 2. Integration Testing

Use in your test suite:

```go
func TestMyApp(t *testing.T) {
    config := embedspicedb.Config{
        SchemaFiles:  []string{"../testdata/schema.zed"},
        GRPCAddress:  ":0", // Use random port
        PresharedKey: "test-key",
    }
    
    server, err := embedspicedb.New(config)
    require.NoError(t, err)
    
    ctx := context.Background()
    require.NoError(t, server.Start(ctx))
    defer server.Stop()
    
    conn, err := server.Client(ctx)
    require.NoError(t, err)
    
    // Run your tests...
}
```

#### 3. Hot Reload During Development

Watch schema files and automatically reload:

```go
config := embedspicedb.Config{
    SchemaFiles:   []string{"./schema.zed"},
    WatchDebounce: 500 * time.Millisecond, // Debounce rapid changes
}

server, _ := embedspicedb.New(config)

// Get notified when schema reloads
server.OnSchemaReloaded(func(err error) {
    if err != nil {
        log.Printf("Schema reload failed: %v", err)
    } else {
        log.Println("Schema reloaded successfully!")
    }
})

server.Start(context.Background())
```

#### 4. Programmatic Schema Updates

Manually reload schema when needed:

```go
// After modifying schema files programmatically
err := server.ReloadSchema(ctx)
if err != nil {
    log.Printf("Failed to reload: %v", err)
}
```

#### 5. Multiple Schema Files

Combine multiple schema files:

```go
config := embedspicedb.Config{
    SchemaFiles: []string{
        "./base.zed",
        "./extensions.zed",
        "./custom.zed",
    },
}
```

### Using with SpiceDB Client Libraries

#### Go Client (authzed-go)

```go
import (
    v1 "github.com/authzed/authzed-go/v1"
    "github.com/akoserwal/embedspicedb"
)

// Get connection from embedded server
conn, _ := server.Client(ctx)

// Create SpiceDB client
client := v1.NewClient(conn, "dev-key")

// Use SpiceDB APIs
client.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{...})
client.CheckPermission(ctx, &v1.CheckPermissionRequest{...})
```

#### gRPC Clients (Any Language)

Since `embedspicedb` exposes a standard gRPC interface, you can use any gRPC client:

```python
# Python example
import grpc
from authzed.api.v1 import schema_service_pb2_grpc

# Connect to embedded server
channel = grpc.insecure_channel('localhost:50051')
stub = schema_service_pb2_grpc.SchemaServiceStub(channel)

# Use SpiceDB APIs
response = stub.ReadSchema(schema_service_pb2.ReadSchemaRequest())
```

### Error Handling

Always handle errors properly:

```go
server, err := embedspicedb.New(config)
if err != nil {
    return fmt.Errorf("failed to create server: %w", err)
}

ctx := context.Background()
if err := server.Start(ctx); err != nil {
    return fmt.Errorf("failed to start server: %w", err)
}

conn, err := server.Client(ctx)
if err != nil {
    return fmt.Errorf("failed to get client: %w", err)
}
```

### Graceful Shutdown

Ensure proper cleanup:

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

server.Start(ctx)

// Handle shutdown signals
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

<-sigChan
log.Println("Shutting down...")
server.Stop()
```

## Examples

See `example_test.go` and `demo/main.go` for more examples including:
- Basic setup
- Custom configuration
- Manual schema reload
- Starting without schema files
- Complete working examples with SpiceDB clients

## Requirements

- Go 1.25.5 or later
- For basic usage with memdb: No additional requirements (standalone)
- For persistent datastores (PostgreSQL/MySQL): Requires SpiceDB source code access

## Development Setup

### Standalone Usage (Recommended)

For development with the in-memory datastore, no special setup is needed:

```bash
go get github.com/akoserwal/embedspicedb
go get github.com/authzed/authzed-go/v1
```

The library is now standalone and includes all necessary packages.

### Using Persistent Datastores

If you need PostgreSQL or MySQL support, you'll need access to SpiceDB source code:

```bash
# Add replace directive to your go.mod
go mod edit -replace github.com/authzed/spicedb=/path/to/spicedb
go mod tidy
```

Or if SpiceDB is in a sibling directory:

```bash
go mod edit -replace github.com/authzed/spicedb=../spicedb
go mod tidy
```

**Note:** Persistent datastore support is only available when using embedspicedb within the SpiceDB module context. For standalone usage, use the in-memory datastore (memdb).

## Limitations

- **Single Node**: Cannot be used with multi-node dispatch (dispatch server disabled)
- **Development Defaults**: Defaults to in-memory datastore (memdb) for development
- **Standalone Mode**: PostgreSQL/MySQL support requires SpiceDB source code access
- **In-Memory Only (Standalone)**: When used standalone, only memdb is available. Data is lost on restart.

## Troubleshooting

### Common Issues

#### "use of internal package not allowed"

**Problem:** You're trying to use PostgreSQL/MySQL in standalone mode.

**Solution:** Use memdb for development, or set up SpiceDB source code access for persistent datastores.

#### "failed to load initial schema"

**Problem:** Schema file not found or invalid.

**Solution:** 
- Check that schema file paths are correct
- Verify schema syntax is valid Zed
- Check file permissions

#### "connection refused" or "can't assign requested address"

**Problem:** Port already in use or invalid address.

**Solution:**
- Use `:0` for random port assignment
- Check if port is already in use: `lsof -i :50051`
- Use a different port number

#### Schema not reloading

**Problem:** File watcher not detecting changes.

**Solution:**
- Check that schema files are in watched directories
- Increase `WatchDebounce` if files are being edited rapidly
- Manually call `ReloadSchema()` if needed

### Getting Help

- Check the [Architecture Documentation](./docs/ARCHITECTURE.md) for detailed information
- Review [SpiceDB Documentation](https://authzed.com/docs)
- See `demo/main.go` for a complete working example

## License

Apache License 2.0

## Architecture

For detailed architecture documentation, see [ARCHITECTURE.md](./docs/ARCHITECTURE.md), which includes:
- Component breakdown and data flow
- How hot reload works
- Gaps and limitations
- Comparison with real SpiceDB
- Confidence assessment for different use cases

## Gaps and Improvements

For a comprehensive analysis of gaps, limitations, and improvement opportunities, see [GAPS.md](./docs/GAPS.md), which includes:
- Testing gaps and recommendations
- Functionality gaps (health checks, TLS, metrics)
- Error handling improvements
- Code quality enhancements
- Documentation needs
- Security considerations
- Performance optimizations

## See Also

- [SpiceDB Documentation](https://authzed.com/docs)
- [Schema DSL Reference](https://authzed.com/docs/reference/schema-lang)
- [Architecture Documentation](./docs/ARCHITECTURE.md)
