## Embedding SpiceDB in Your App: `embedspicedb` for Fast Local Authorization, Demos, and Tests

Modern applications rarely have “just a database.” They have **authorization rules**: who can view a document, who can approve an expense, who can administer an org. When those rules get complex, teams often reach for a dedicated permissions system like **SpiceDB**.

But there’s a friction point: running a full permission service in every dev environment, integration test, demo, or ephemeral preview can be **slow** and **operationally heavy**.

This is the motivation behind **`embedspicedb`**: a small Go library that lets you run a **real SpiceDB server** *inside your process*, backed by an in-memory datastore, with optional **schema hot reload** and an optional **health endpoint**.

### Why this concept is useful

- **Fast local development**: run a “real” permissions engine without provisioning Postgres/MySQL, deploying another service, or wiring auth tokens.
- **Integration tests that feel like production**: exercise the actual gRPC APIs (schema, relationships, permission checks) with minimal setup.
- **Demos and prototypes**: ship a single binary that includes both your app and SpiceDB.
- **Preview environments**: spin up ephemeral instances quickly (with in-memory state).

### What you get (and what you don’t)

**You get**
- A real embedded SpiceDB server (`RunnableServer`) with the full gRPC API surface.
- In-memory datastore (`memdb`) for fast, disposable state.
- Optional schema file watching + debounced reload.
- Programmatic health checks + optional HTTP `/healthz` endpoint.

**You don’t get (by default)**
- Persistent datastore support in standalone mode (Postgres/MySQL are intentionally not wired in this repo’s standalone build).
- Distributed dispatch / multi-node clustering.
- Production-grade TLS/authz front-door and observability by default.

### Diagram: high-level architecture (Excalidraw)

Open `docs/blog-architecture.excalidraw` in Excalidraw:
- Local file: `docs/blog-architecture.excalidraw`

### Diagram: schema hot reload flow (Excalidraw)

Open `docs/blog-hot-reload.excalidraw` in Excalidraw:
- Local file: `docs/blog-hot-reload.excalidraw`

---

## How it works (in plain language)

At startup:
1. `embedspicedb.New(Config)` creates an embedded server with an in-memory datastore.
2. `Start(ctx)` configures and runs a SpiceDB server in a goroutine.
3. The server dials itself via gRPC and keeps a reusable `*grpc.ClientConn`.
4. If schema files are configured, it loads them via the **SchemaService** and optionally starts a file watcher to reload on changes.

At runtime:
- Your application uses `server.Client(ctx)` to obtain a gRPC connection and calls the normal Authzed/SpiceDB APIs (e.g. `CheckPermission`, `WriteRelationships`).

---

## How to use it

### 1) Install

```bash
go get github.com/akoserwal/embedspicedb
```

### 2) Create a schema file

Example `schema.zed`:

```zed
definition user {}

definition document {
  relation reader: user
  relation writer: user

  permission read = reader
  permission write = writer
}
```

### 3) Start the embedded server

```go
package main

import (
  "context"
  "log"
  "time"

  "github.com/akoserwal/embedspicedb"
  v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func main() {
  ctx := context.Background()

  cfg := embedspicedb.Config{
    SchemaFiles:   []string{"./schema.zed"},
    GRPCAddress:   "127.0.0.1:50051",
    PresharedKey:  "dev-key",
    WatchDebounce: 300 * time.Millisecond,

    // Optional: health endpoint for readiness/liveness probes
    HealthCheckEnabled: true,
    // Bind to a random free port; then read it from server.HealthCheckHTTPAddr()
    HealthCheckAddress: "127.0.0.1:0",
  }

  s, err := embedspicedb.New(cfg)
  if err != nil {
    log.Fatal(err)
  }
  defer s.Stop()

  if err := s.Start(ctx); err != nil {
    log.Fatal(err)
  }

  log.Printf("health endpoint: http://%s/healthz", s.HealthCheckHTTPAddr())

  conn, err := s.Client(ctx)
  if err != nil {
    log.Fatal(err)
  }

  perms := v1.NewPermissionsServiceClient(conn)
  // ... use perms.CheckPermission / perms.WriteRelationships ...
  _ = perms
}
```

### 4) Write relationships and check permissions

```go
_, err := perms.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
  Updates: []*v1.RelationshipUpdate{
    {
      Operation: v1.RelationshipUpdate_OPERATION_CREATE,
      Relationship: &v1.Relationship{
        Resource: &v1.ObjectReference{ObjectType: "document", ObjectId: "doc1"},
        Relation:  "reader",
        Subject: &v1.SubjectReference{
          Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "alice"},
        },
      },
    },
  },
})
if err != nil { /* handle */ }

check, err := perms.CheckPermission(ctx, &v1.CheckPermissionRequest{
  Resource:    &v1.ObjectReference{ObjectType: "document", ObjectId: "doc1"},
  Permission:  "read",
  Subject:     &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "alice"}},
})
if err != nil { /* handle */ }

// check.Permissionship == HAS_PERMISSION means “allowed”
```

---

## When to use this (and when not to)

Use `embedspicedb` when you want:
- **Real API integration** without deploying a full permissions stack
- **Fast feedback loops** for dev/test
- **Simple demos** that still use the real SpiceDB semantics

Avoid using it as-is for:
- Production multi-node deployments
- Workloads requiring durable persistence (standalone mode is memdb-only)
- High-scale latency-critical paths (you’ll want proper dispatch tuning, caching, and observability)

---

## Conclusion

`embedspicedb` is a pragmatic bridge between “toy authorization mocks” and “full service deployment.” It lets you run **real SpiceDB semantics** inside your app for development, tests, and demos—without dragging in all the operational complexity.

If you later need production features (persistence, TLS, observability, multi-node), you can graduate to a full SpiceDB deployment while keeping the same core schema + API calls.


