package embedspicedb_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	. "github.com/akoserwal/embedspicedb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config with defaults",
			config: Config{
				GRPCAddress:  ":0",
				PresharedKey: "test-key",
			},
			wantErr: false,
		},
		{
			name:    "empty config uses defaults",
			config:  Config{},
			wantErr: false,
		},
		{
			name: "config with memdb",
			config: Config{
				DatastoreType: "memdb",
				GRPCAddress:   ":0",
			},
			wantErr: false,
		},
		{
			name: "config with postgres returns error",
			config: Config{
				DatastoreType: "postgres",
				DatastoreURI:  "postgres://test",
				GRPCAddress:   ":0",
			},
			wantErr: true,
		},
		{
			name: "config with mysql returns error",
			config: Config{
				DatastoreType: "mysql",
				DatastoreURI:  "mysql://test",
				GRPCAddress:   ":0",
			},
			wantErr: true,
		},
		{
			name: "invalid datastore type",
			config: Config{
				DatastoreType: "invalid",
				GRPCAddress:   ":0",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config
			if cfg.GRPCAddress == ":0" || cfg.GRPCAddress == "127.0.0.1:0" {
				cfg.GRPCAddress = getFreePort(t)
			}
			server, err := New(cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, server)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, server)
				if server != nil {
					// Cleanup
					server.Stop()
				}
			}
		})
	}
}

func TestEmbeddedServer_Start(t *testing.T) {
	t.Run("start successfully", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		assert.NoError(t, err)
	})

	t.Run("start without schema files", func(t *testing.T) {
		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		assert.NoError(t, err)
	})

	t.Run("start twice returns error", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		// Try to start again
		err = server.Start(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already started")
	})

	t.Run("start with invalid schema file", func(t *testing.T) {
		config := Config{
			SchemaFiles:  []string{"/nonexistent/file.zed"},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		// Start should succeed, but schema load will fail
		assert.NoError(t, err)
	})

	t.Run("start with HTTP enabled", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			HTTPEnabled:  true,
			HTTPAddress:  ":0",
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		assert.NoError(t, err)
	})
}

func TestEmbeddedServer_Stop(t *testing.T) {
	t.Run("stop started server", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		err = server.Stop()
		assert.NoError(t, err)
	})

	t.Run("stop stopped server", func(t *testing.T) {
		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)

		// Stop without starting
		err = server.Stop()
		assert.NoError(t, err)

		// Stop again
		err = server.Stop()
		assert.NoError(t, err)
	})

	t.Run("stop multiple times", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		err = server.Stop()
		assert.NoError(t, err)

		// Stop again
		err = server.Stop()
		assert.NoError(t, err)
	})
}

func TestEmbeddedServer_Client(t *testing.T) {
	t.Run("get client after start", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, conn)

		// Verify connection works
		schemaClient := v1.NewSchemaServiceClient(conn)
		_, err = schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
		assert.NoError(t, err)
	})

	t.Run("get client before start", func(t *testing.T) {
		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		conn, err := server.Client(ctx)
		assert.Error(t, err)
		assert.Nil(t, conn)
		assert.Contains(t, err.Error(), "not started")
	})

	t.Run("get client after stop", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		err = server.Stop()
		require.NoError(t, err)

		conn, err := server.Client(ctx)
		assert.Error(t, err)
		assert.Nil(t, conn)
	})
}

func TestEmbeddedServer_ReloadSchema(t *testing.T) {
	t.Run("reload schema successfully", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		err = server.ReloadSchema(ctx)
		assert.NoError(t, err)
	})

	t.Run("reload schema before start", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.ReloadSchema(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not started")
	})

	t.Run("reload schema without schema files", func(t *testing.T) {
		config := Config{
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		err = server.ReloadSchema(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no schema files")
	})

	t.Run("reload schema with invalid file", func(t *testing.T) {
		config := Config{
			SchemaFiles:  []string{"/nonexistent/file.zed"},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		err = server.ReloadSchema(ctx)
		assert.Error(t, err)
	})
}

func TestEmbeddedServer_OnSchemaReloaded(t *testing.T) {
	t.Run("callback called on manual reload", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		var callbackCalled bool
		var callbackError error

		server.OnSchemaReloaded(func(err error) {
			callbackCalled = true
			callbackError = err
		})

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		err = server.ReloadSchema(ctx)
		require.NoError(t, err)

		// Give callback time to execute
		time.Sleep(100 * time.Millisecond)

		assert.True(t, callbackCalled)
		assert.NoError(t, callbackError)
	})

	t.Run("multiple callbacks", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		var callback1Called, callback2Called bool

		server.OnSchemaReloaded(func(err error) {
			callback1Called = true
		})

		server.OnSchemaReloaded(func(err error) {
			callback2Called = true
		})

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		err = server.ReloadSchema(ctx)
		require.NoError(t, err)

		// Give callbacks time to execute
		time.Sleep(100 * time.Millisecond)

		assert.True(t, callback1Called)
		assert.True(t, callback2Called)
	})
}

func TestEmbeddedServer_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent client access", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		var wg sync.WaitGroup
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				conn, err := server.Client(ctx)
				assert.NoError(t, err)
				if conn != nil {
					schemaClient := v1.NewSchemaServiceClient(conn)
					_, _ = schemaClient.ReadSchema(ctx, &v1.ReadSchemaRequest{})
				}
			}()
		}

		wg.Wait()
	})

	t.Run("concurrent reload schema", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)
		defer server.Stop()

		ctx := context.Background()
		err = server.Start(ctx)
		require.NoError(t, err)

		var wg sync.WaitGroup
		numGoroutines := 5

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = server.ReloadSchema(ctx)
			}()
		}

		wg.Wait()
	})
}

func TestEmbeddedServer_Lifecycle(t *testing.T) {
	t.Run("full lifecycle", func(t *testing.T) {
		tmpFile := createTempSchemaFile(t)
		defer os.Remove(tmpFile)

		config := Config{
			SchemaFiles:  []string{tmpFile},
			GRPCAddress:  getFreePort(t),
			PresharedKey: "test-key",
		}

		server, err := New(config)
		require.NoError(t, err)

		ctx := context.Background()

		// Start
		err = server.Start(ctx)
		require.NoError(t, err)

		// Get client
		conn, err := server.Client(ctx)
		require.NoError(t, err)
		assert.NotNil(t, conn)

		// Reload schema
		err = server.ReloadSchema(ctx)
		assert.NoError(t, err)

		// Stop
		err = server.Stop()
		assert.NoError(t, err)

		// Verify stopped
		conn, err = server.Client(ctx)
		assert.Error(t, err)
		assert.Nil(t, conn)
	})
}

func TestCreateDatastore(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "memdb default",
			config: Config{
				DatastoreType: "",
			},
			wantErr: false,
		},
		{
			name: "memdb explicit",
			config: Config{
				DatastoreType: "memdb",
			},
			wantErr: false,
		},
		{
			name: "postgres not available",
			config: Config{
				DatastoreType: "postgres",
				DatastoreURI:  "postgres://test",
			},
			wantErr: true,
		},
		{
			name: "postgresql not available",
			config: Config{
				DatastoreType: "postgresql",
				DatastoreURI:  "postgres://test",
			},
			wantErr: true,
		},
		{
			name: "mysql not available",
			config: Config{
				DatastoreType: "mysql",
				DatastoreURI:  "mysql://test",
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			config: Config{
				DatastoreType: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// createDatastore is intentionally unexported; validate behavior via New().
			s, err := New(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, s)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, s)
				if s != nil {
					_ = s.Stop()
				}
			}
		})
	}
}
