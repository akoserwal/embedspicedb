# Migration Guide: Moving embedspicedb to Separate Repository

This document explains how the `embedspicedb` package was moved from `pkg/embedspicedb` in the SpiceDB repository to a separate repository.

## Repository Structure

The new repository is located at `/Users/akoserwa/kessel/spicedb-exp/embedspicedb` and contains:

- `config.go` - Configuration options
- `server.go` - Main server wrapper
- `watcher.go` - File watcher for hot reload
- `reloader.go` - Schema reload logic
- `example_test.go` - Usage examples
- `README.md` - Documentation
- `go.mod` - Go module definition
- `LICENSE` - Apache 2.0 license

## Important Notes

### Internal Package Access

This library currently uses SpiceDB's internal packages:
- `github.com/authzed/spicedb/internal/datastore/memdb`
- `github.com/authzed/spicedb/internal/logging`
- `github.com/authzed/spicedb/pkg/cmd/server`
- `github.com/authzed/spicedb/pkg/cmd/util`
- `github.com/authzed/spicedb/pkg/datastore`
- `github.com/authzed/spicedb/pkg/validationfile`

Because Go's internal packages cannot be imported from outside the module, users of this library must:

1. Have access to the SpiceDB source code
2. Use a `replace` directive in their `go.mod`:

```go
replace github.com/authzed/spicedb => /path/to/spicedb
```

### Future Considerations

For this library to be truly standalone and distributable:

1. **Option A**: SpiceDB could expose the necessary APIs as public packages
2. **Option B**: This library could be maintained as part of the SpiceDB repository
3. **Option C**: Use SpiceDB's public APIs only (may require significant refactoring)

## Using the Library

### Local Development

```bash
cd /path/to/your/project
go mod edit -replace github.com/authzed/spicedb=/path/to/spicedb
go mod edit -replace github.com/authzed/embedspicedb=/path/to/embedspicedb
go get github.com/authzed/embedspicedb
go mod tidy
```

### Example Usage

```go
import "github.com/authzed/embedspicedb"

config := embedspicedb.Config{
    SchemaFiles:  []string{"./schema.zed"},
    GRPCAddress:  ":50051",
    PresharedKey: "dev-key",
}

server, err := embedspicedb.New(config)
// ... rest of usage
```

## Next Steps

1. Decide on the distribution strategy (see Future Considerations above)
2. If keeping as separate repo, consider:
   - Publishing to a public repository (GitHub, etc.)
   - Setting up CI/CD
   - Versioning strategy
   - Documentation site

