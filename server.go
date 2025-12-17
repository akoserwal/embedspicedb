package embedspicedb

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/akoserwal/embedspicedb/internal/datastore/memdb"
	"github.com/akoserwal/embedspicedb/internal/healthhttp"
	log "github.com/akoserwal/embedspicedb/internal/logging"
	"github.com/authzed/spicedb/pkg/cmd/server"
	"github.com/authzed/spicedb/pkg/cmd/util"
	"github.com/authzed/spicedb/pkg/datastore"
)

// EmbeddedServer wraps a SpiceDB server with hot reload capabilities.
type EmbeddedServer struct {
	config          Config
	server          server.RunnableServer
	datastore       datastore.Datastore
	reloader        *SchemaReloader
	watcher         *FileWatcher
	conn            *grpc.ClientConn
	reloadCallbacks []func(error)
	healthSrv       *healthhttp.Server
	mu              sync.RWMutex
	started         bool
	startTime       *time.Time
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// New creates a new embedded SpiceDB server with the given configuration.
func New(config Config) (*EmbeddedServer, error) {
	config.WithDefaults()
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create datastore based on configuration
	ds, err := createDatastore(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create datastore: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	es := &EmbeddedServer{
		config:          config,
		datastore:       ds,
		reloadCallbacks: make([]func(error), 0),
		ctx:             ctx,
		cancel:          cancel,
	}

	return es, nil
}

// createDatastore creates the appropriate datastore based on configuration.
func createDatastore(ctx context.Context, config Config) (datastore.Datastore, error) {
	datastoreType := strings.ToLower(config.DatastoreType)
	if datastoreType == "" {
		datastoreType = "memdb"
	}

	switch datastoreType {
	case "memdb":
		// Create in-memory datastore
		return memdb.NewMemdbDatastore(
			config.WatchBufferLength,
			config.RevisionQuantization,
			config.GCWindow,
		)

	case "postgres", "postgresql":
		return nil, fmt.Errorf("PostgreSQL datastore is not available in standalone embedspicedb. Use memdb for development, or use embedspicedb within the SpiceDB module context for persistent datastores")

	case "mysql":
		return nil, fmt.Errorf("MySQL datastore is not available in standalone embedspicedb. Use memdb for development, or use embedspicedb within the SpiceDB module context for persistent datastores")

	default:
		return nil, fmt.Errorf("unsupported datastore type: %s (supported: memdb, postgres, mysql)", config.DatastoreType)
	}
}

// Start starts the server and begins watching schema files for changes.
func (es *EmbeddedServer) Start(ctx context.Context) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.started {
		return fmt.Errorf("server is already started")
	}

	// Create server configuration
	serverConfig := server.NewConfigWithOptionsAndDefaults(
		server.WithDatastore(es.datastore),
		server.WithPresharedSecureKey(es.config.PresharedKey),
		server.WithGRPCServer(util.GRPCServerConfig{
			Address: es.config.GRPCAddress,
			Network: "tcp",
			Enabled: true,
		}),
		server.WithHTTPGateway(util.HTTPServerConfig{
			HTTPEnabled: es.config.HTTPEnabled,
			HTTPAddress: es.config.HTTPAddress,
		}),
		server.WithMetricsAPI(util.HTTPServerConfig{
			HTTPEnabled: false,
		}),
		server.WithDispatchServer(util.GRPCServerConfig{
			Enabled: false,
		}),
	)

	// Complete server configuration
	srv, err := serverConfig.Complete(ctx)
	if err != nil {
		return fmt.Errorf("failed to complete server configuration: %w", err)
	}
	es.server = srv

	// Start server in background
	es.wg.Add(1)
	go func() {
		defer es.wg.Done()
		if err := es.server.Run(es.ctx); err != nil {
			log.Ctx(es.ctx).Error().Err(err).Msg("server error")
		}
	}()

	// Get client connection with retry/backoff
	conn, err := es.dialWithRetry(ctx)
	if err != nil {
		es.cancel()
		es.wg.Wait()
		return fmt.Errorf("failed to dial server: %w", err)
	}
	es.conn = conn

	// Create schema reloader
	es.reloader = NewSchemaReloader(conn, es.config.SchemaFiles)

	// Initial schema load if files are provided
	if len(es.config.SchemaFiles) > 0 {
		if err := es.reloader.Reload(ctx); err != nil {
			log.Ctx(ctx).Warn().Err(err).Msg("failed to load initial schema")
		}
	}

	// Start file watcher if schema files are configured
	if len(es.config.SchemaFiles) > 0 {
		watcher, err := NewFileWatcher(es.config.SchemaFiles, es.config.WatchDebounce, func() error {
			return es.ReloadSchema(ctx)
		})
		if err != nil {
			// File watching is an optional convenience; don't fail server startup if it can't be created.
			log.Ctx(ctx).Warn().Err(err).Strs("files", es.config.SchemaFiles).Msg("failed to create file watcher; hot reload disabled")
		} else if err := watcher.Start(); err != nil {
			// Ensure we don't leak file descriptors if Start partially succeeded.
			_ = watcher.Stop()
			log.Ctx(ctx).Warn().Err(err).Strs("files", es.config.SchemaFiles).Msg("failed to start file watcher; hot reload disabled")
		} else {
			es.watcher = watcher
			log.Ctx(ctx).Info().Strs("files", es.config.SchemaFiles).Msg("watching schema files for changes")
		}
	}

	// Start health check server if enabled
	now := time.Now()
	es.startTime = &now
	if err := es.startHealthCheckServer(ctx); err != nil {
		log.Ctx(ctx).Warn().Err(err).Msg("failed to start health check server")
		// Don't fail server startup if health check server fails
	}

	es.started = true
	log.Ctx(ctx).Info().
		Str("grpc_address", es.config.GRPCAddress).
		Bool("http_enabled", es.config.HTTPEnabled).
		Bool("health_check_enabled", es.config.HealthCheckEnabled).
		Str("health_check_address", es.config.HealthCheckAddress).
		Msg("embedded SpiceDB server started")

	return nil
}

// Stop stops the server and file watchers.
func (es *EmbeddedServer) Stop() error {
	es.mu.Lock()
	defer es.mu.Unlock()

	if !es.started {
		return nil
	}

	log.Ctx(es.ctx).Info().Msg("stopping embedded SpiceDB server")

	// Stop health check server
	if err := es.stopHealthCheckServer(es.ctx); err != nil {
		log.Ctx(es.ctx).Warn().Err(err).Msg("error stopping health check server")
	}

	// Stop file watcher
	if es.watcher != nil {
		if err := es.watcher.Stop(); err != nil {
			log.Ctx(es.ctx).Warn().Err(err).Msg("error stopping file watcher")
		}
	}

	// Close connection
	if es.conn != nil {
		if err := es.conn.Close(); err != nil {
			log.Ctx(es.ctx).Warn().Err(err).Msg("error closing connection")
		}
	}

	// Cancel context to stop server
	es.cancel()

	// Wait for server to stop
	es.wg.Wait()

	// Close datastore
	if es.datastore != nil {
		if err := es.datastore.Close(); err != nil {
			log.Ctx(es.ctx).Warn().Err(err).Msg("error closing datastore")
		}
	}

	es.started = false
	log.Ctx(es.ctx).Info().Msg("embedded SpiceDB server stopped")

	return nil
}

// HealthCheckHTTPAddr returns the bound address for the HTTP health check server, if enabled and started.
// Returns empty string if the health check server is disabled or not yet started.
func (es *EmbeddedServer) HealthCheckHTTPAddr() string {
	es.mu.RLock()
	defer es.mu.RUnlock()
	if es.healthSrv == nil {
		return ""
	}
	return es.healthSrv.Addr()
}

// Client returns a gRPC client connection to the server.
func (es *EmbeddedServer) Client(ctx context.Context) (*grpc.ClientConn, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	if !es.started {
		return nil, fmt.Errorf("server is not started")
	}

	if es.conn == nil {
		return nil, fmt.Errorf("no connection available")
	}

	return es.conn, nil
}

// ReloadSchema manually reloads schema files.
func (es *EmbeddedServer) ReloadSchema(ctx context.Context) error {
	es.mu.RLock()
	if !es.started {
		es.mu.RUnlock()
		return fmt.Errorf("server is not started")
	}

	if es.reloader == nil {
		es.mu.RUnlock()
		return fmt.Errorf("no schema reloader available")
	}

	reloader := es.reloader
	es.mu.RUnlock()

	// Perform reload outside lock to avoid blocking other operations
	err := reloader.Reload(ctx)

	// Get callbacks under lock, then invoke outside lock
	es.mu.RLock()
	callbacks := make([]func(error), len(es.reloadCallbacks))
	copy(callbacks, es.reloadCallbacks)
	es.mu.RUnlock()

	for _, callback := range callbacks {
		callback(err)
	}

	return err
}

// OnSchemaReloaded registers a callback function that will be called whenever
// the schema is reloaded (either automatically via file watching or manually).
func (es *EmbeddedServer) OnSchemaReloaded(callback func(error)) {
	es.mu.Lock()
	defer es.mu.Unlock()

	es.reloadCallbacks = append(es.reloadCallbacks, callback)
}

// dialWithRetry attempts to connect to the gRPC server with exponential backoff.
func (es *EmbeddedServer) dialWithRetry(ctx context.Context) (*grpc.ClientConn, error) {
	const (
		maxRetries     = 10
		initialBackoff = 10 * time.Millisecond
		maxBackoff     = 500 * time.Millisecond
	)

	var lastErr error
	backoff := initialBackoff

	for i := 0; i < maxRetries; i++ {
		conn, err := es.server.GRPCDialContext(ctx, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			return conn, nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-es.ctx.Done():
			return nil, es.ctx.Err()
		case <-time.After(backoff):
			// Exponential backoff with cap
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}
