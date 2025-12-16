# SpiceDB Embedded Server Demo

This demo showcases the key features of the embedspicedb library.

## Running the Demo

From the `embedspicedb` directory:

```bash
cd demo
go run main.go
```

Or from the project root:

```bash
go run demo/main.go
```

## What the Demo Shows

1. **Server Setup**: Creates and starts an embedded SpiceDB server with schema file watching
2. **Schema Reading**: Reads and displays the loaded schema
3. **Writing Relationships**: Creates relationships between users and documents
4. **Permission Checking**: Checks various permissions for different users
5. **Hot Reload**: Demonstrates schema hot-reload by updating the schema file
6. **Reading Relationships**: Reads back all created relationships

## Demo Flow

1. Creates a temporary schema file with basic document permissions
2. Starts the embedded SpiceDB server
3. Writes relationships:
   - `alice` is a `reader` of `document1`
   - `bob` is a `writer` of `document1`
4. Checks permissions for different users
5. Updates the schema to add an `owner` relation and `admin` permission
6. Demonstrates hot reload (automatic schema reload)
7. Creates new relationships using the updated schema
8. Reads all relationships back

## Expected Output

The demo will show:
- Server configuration and startup
- Schema content
- Relationship write operations
- Permission check results (✅ ALLOWED / ❌ DENIED)
- Hot reload confirmation
- All relationships in the system

## Requirements

- Go 1.25.5 or later
- SpiceDB source code (for internal packages)
- The demo uses the SpiceDB package from `../spicedb` (configured via go.mod replace directive)

