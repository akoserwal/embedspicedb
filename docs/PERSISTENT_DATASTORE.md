# Persistent Datastore Support

## Overview

`embedspicedb` now supports persistent datastores (PostgreSQL and MySQL) in addition to the in-memory datastore (memdb). This makes it suitable for production workloads requiring data persistence.

## Changes Made

### 1. Configuration Updates (`config.go`)

Added two new configuration fields:

- **`DatastoreType`**: Specifies the type of datastore to use
  - Options: `"memdb"` (default), `"postgres"`/`"postgresql"`, `"mysql"`
  
- **`DatastoreURI`**: Connection URI for persistent datastores
  - Required when using PostgreSQL or MySQL
  - PostgreSQL format: `"postgres://user:password@host:port/database?sslmode=disable"`
  - MySQL format: `"user:password@tcp(host:port)/database?parseTime=true"`

### 2. Server Implementation (`server.go`)

- Added `createDatastore()` function that creates the appropriate datastore based on configuration
- Supports three datastore types:
  - **memdb**: In-memory (default, for development)
  - **postgres**: PostgreSQL (for production)
  - **mysql**: MySQL (for production)
- Properly configures datastore options (WatchBufferLength, RevisionQuantization, GCWindow) for all types

### 3. Documentation Updates

- Updated README.md with persistent datastore examples
- Updated ARCHITECTURE.md to reflect production readiness
- Added example_persistent_test.go with usage examples

## Usage Examples

### Development (In-Memory)

```go
config := embedspicedb.Config{
    SchemaFiles:   []string{"./schema.zed"},
    GRPCAddress:   ":50051",
    PresharedKey:  "dev-key",
    // DatastoreType defaults to "memdb"
}
```

### Production (PostgreSQL)

```go
config := embedspicedb.Config{
    SchemaFiles:   []string{"./schema.zed"},
    GRPCAddress:   ":50051",
    PresharedKey:  "prod-key",
    DatastoreType: "postgres",
    DatastoreURI:  "postgres://user:password@localhost:5432/spicedb?sslmode=disable",
    RevisionQuantization: 5 * time.Second,
    GCWindow:      24 * time.Hour,
}
```

### Production (MySQL)

```go
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

## Benefits

✅ **Production Ready**: Can now be used in production with persistent datastores  
✅ **Data Persistence**: Data survives application restarts  
✅ **Backward Compatible**: Defaults to memdb, existing code continues to work  
✅ **Flexible**: Choose the right datastore for your use case  
✅ **Same API**: No code changes needed when switching datastores  

## Migration Path

### From memdb to PostgreSQL/MySQL

1. Set up your PostgreSQL or MySQL database
2. Update configuration:
   ```go
   config.DatastoreType = "postgres" // or "mysql"
   config.DatastoreURI = "your-connection-uri"
   ```
3. No code changes needed - same API, same behavior!

### Database Setup

**PostgreSQL:**
```sql
CREATE DATABASE spicedb;
-- SpiceDB will run migrations automatically on first connection
```

**MySQL:**
```sql
CREATE DATABASE spicedb;
-- SpiceDB will run migrations automatically on first connection
-- Ensure parseTime=true is in connection string
```

## Important Notes

1. **Database Migrations**: SpiceDB will automatically run required migrations on first connection
2. **MySQL Requirement**: MySQL connection strings must include `parseTime=true`
3. **CockroachDB**: PostgreSQL datastore is also compatible with CockroachDB
4. **Performance**: Persistent datastores have different performance characteristics than memdb
5. **Connection Pooling**: PostgreSQL and MySQL datastores handle connection pooling internally

## Testing

All existing tests continue to pass. The default memdb behavior is unchanged, ensuring backward compatibility.

## Architecture Impact

- **Development**: Still uses memdb by default (fast, simple)
- **Production**: Can use PostgreSQL/MySQL (persistent, reliable)
- **Same Codebase**: Uses same SpiceDB datastore implementations as standalone SpiceDB
- **Same Features**: All SpiceDB features work identically regardless of datastore type

