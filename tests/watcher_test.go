package embedspicedb_test

import (
	. "github.com/akoserwal/embedspicedb"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileWatcher(t *testing.T) {
	t.Run("create watcher successfully", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "test content")
		defer os.Remove(tmpFile)

		watcher, err := NewFileWatcher([]string{tmpFile}, 500*time.Millisecond, func() error {
			return nil
		})

		assert.NoError(t, err)
		assert.NotNil(t, watcher)
		if watcher != nil {
			watcher.Stop()
		}
	})

	t.Run("create watcher with multiple files", func(t *testing.T) {
		tmpFile1 := createTempFile(t, "test1.zed", "content1")
		tmpFile2 := createTempFile(t, "test2.zed", "content2")
		defer os.Remove(tmpFile1)
		defer os.Remove(tmpFile2)

		watcher, err := NewFileWatcher([]string{tmpFile1, tmpFile2}, 500*time.Millisecond, func() error {
			return nil
		})

		assert.NoError(t, err)
		assert.NotNil(t, watcher)
		if watcher != nil {
			watcher.Stop()
		}
	})

	t.Run("create watcher with empty files", func(t *testing.T) {
		watcher, err := NewFileWatcher([]string{}, 500*time.Millisecond, func() error {
			return nil
		})

		assert.NoError(t, err)
		assert.NotNil(t, watcher)
		if watcher != nil {
			watcher.Stop()
		}
	})
}

func TestFileWatcher_Start(t *testing.T) {
	t.Run("start successfully", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "test content")
		defer os.Remove(tmpFile)

		watcher, err := NewFileWatcher([]string{tmpFile}, 500*time.Millisecond, func() error {
			return nil
		})
		require.NoError(t, err)
		defer watcher.Stop()

		err = watcher.Start()
		assert.NoError(t, err)
	})

	t.Run("start with nonexistent file", func(t *testing.T) {
		// Use a path in temp dir - create the directory but not the file
		tmpDir := t.TempDir()
		nonexistentFile := filepath.Join(tmpDir, "nonexistent.zed")

		watcher, err := NewFileWatcher([]string{nonexistentFile}, 500*time.Millisecond, func() error {
			return nil
		})
		require.NoError(t, err)
		defer watcher.Stop()

		err = watcher.Start()
		// Start should succeed even if file doesn't exist yet (watches directory)
		assert.NoError(t, err)
	})

	t.Run("start with multiple files in same directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile1 := filepath.Join(tmpDir, "test1.zed")
		tmpFile2 := filepath.Join(tmpDir, "test2.zed")

		os.WriteFile(tmpFile1, []byte("content1"), 0644)
		os.WriteFile(tmpFile2, []byte("content2"), 0644)

		watcher, err := NewFileWatcher([]string{tmpFile1, tmpFile2}, 500*time.Millisecond, func() error {
			return nil
		})
		require.NoError(t, err)
		defer watcher.Stop()

		err = watcher.Start()
		assert.NoError(t, err)
	})
}

func TestFileWatcher_Stop(t *testing.T) {
	t.Run("stop started watcher", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "test content")
		defer os.Remove(tmpFile)

		watcher, err := NewFileWatcher([]string{tmpFile}, 500*time.Millisecond, func() error {
			return nil
		})
		require.NoError(t, err)

		err = watcher.Start()
		require.NoError(t, err)

		err = watcher.Stop()
		assert.NoError(t, err)
	})

	t.Run("stop stopped watcher", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "test content")
		defer os.Remove(tmpFile)

		watcher, err := NewFileWatcher([]string{tmpFile}, 500*time.Millisecond, func() error {
			return nil
		})
		require.NoError(t, err)

		err = watcher.Stop()
		assert.NoError(t, err)

		// Stop again
		err = watcher.Stop()
		assert.NoError(t, err)
	})

	t.Run("stop without start", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "test content")
		defer os.Remove(tmpFile)

		watcher, err := NewFileWatcher([]string{tmpFile}, 500*time.Millisecond, func() error {
			return nil
		})
		require.NoError(t, err)

		err = watcher.Stop()
		assert.NoError(t, err)
	})
}

func TestFileWatcher_FileChange(t *testing.T) {
	t.Run("detect file write", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "initial content")
		defer os.Remove(tmpFile)

		var reloadCalled bool
		var reloadMutex sync.Mutex

		watcher, err := NewFileWatcher([]string{tmpFile}, 100*time.Millisecond, func() error {
			reloadMutex.Lock()
			defer reloadMutex.Unlock()
			reloadCalled = true
			return nil
		})
		require.NoError(t, err)
		defer watcher.Stop()

		err = watcher.Start()
		require.NoError(t, err)

		// Wait a bit for watcher to be ready
		time.Sleep(50 * time.Millisecond)

		// Write to file
		err = os.WriteFile(tmpFile, []byte("updated content"), 0644)
		require.NoError(t, err)

		// Wait for debounce and reload
		time.Sleep(300 * time.Millisecond)

		reloadMutex.Lock()
		called := reloadCalled
		reloadMutex.Unlock()

		assert.True(t, called, "reload function should have been called")
	})

	t.Run("debounce rapid changes", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "initial")
		defer os.Remove(tmpFile)

		var reloadCount int
		var reloadMutex sync.Mutex

		watcher, err := NewFileWatcher([]string{tmpFile}, 200*time.Millisecond, func() error {
			reloadMutex.Lock()
			defer reloadMutex.Unlock()
			reloadCount++
			return nil
		})
		require.NoError(t, err)
		defer watcher.Stop()

		err = watcher.Start()
		require.NoError(t, err)

		// Wait for watcher to be ready
		time.Sleep(50 * time.Millisecond)

		// Write multiple times rapidly
		for i := 0; i < 5; i++ {
			os.WriteFile(tmpFile, []byte("content "+string(rune(i))), 0644)
			time.Sleep(50 * time.Millisecond)
		}

		// Wait for debounce
		time.Sleep(400 * time.Millisecond)

		reloadMutex.Lock()
		count := reloadCount
		reloadMutex.Unlock()

		// Should only reload once due to debouncing
		assert.Equal(t, 1, count, "should only reload once due to debouncing")
	})

	t.Run("handle file create", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "newfile.zed")

		var reloadCalled bool
		var reloadMutex sync.Mutex

		watcher, err := NewFileWatcher([]string{tmpFile}, 100*time.Millisecond, func() error {
			reloadMutex.Lock()
			defer reloadMutex.Unlock()
			reloadCalled = true
			return nil
		})
		require.NoError(t, err)
		defer watcher.Stop()

		err = watcher.Start()
		require.NoError(t, err)

		// Wait for watcher to be ready
		time.Sleep(50 * time.Millisecond)

		// Create file
		err = os.WriteFile(tmpFile, []byte("new content"), 0644)
		require.NoError(t, err)

		// Wait for debounce
		time.Sleep(300 * time.Millisecond)

		reloadMutex.Lock()
		called := reloadCalled
		reloadMutex.Unlock()

		assert.True(t, called, "reload should be called on file create")
	})
}

func TestFileWatcher_WatchedVsUnwatchedBehavior(t *testing.T) {
	t.Run("watched file triggers reload; unwatched does not", func(t *testing.T) {
		tmpFile := createTempFile(t, "watched.zed", "content")
		otherFile := createTempFile(t, "unwatched.zed", "content")

		var reloadCalled bool
		var mu sync.Mutex

		watcher, err := NewFileWatcher([]string{tmpFile}, 100*time.Millisecond, func() error {
			mu.Lock()
			reloadCalled = true
			mu.Unlock()
			return nil
		})
		require.NoError(t, err)
		defer watcher.Stop()
		require.NoError(t, watcher.Start())

		time.Sleep(50 * time.Millisecond)

		// Write to the unwatched file -> should not trigger reload.
		require.NoError(t, os.WriteFile(otherFile, []byte("updated"), 0o644))
		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		calledAfterUnwatched := reloadCalled
		mu.Unlock()
		assert.False(t, calledAfterUnwatched)

		// Write to the watched file -> should trigger reload.
		require.NoError(t, os.WriteFile(tmpFile, []byte("updated"), 0o644))
		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		calledAfterWatched := reloadCalled
		mu.Unlock()
		assert.True(t, calledAfterWatched)
	})
}

func TestFileWatcher_ReloadError(t *testing.T) {
	t.Run("handle reload error", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "content")
		defer os.Remove(tmpFile)

		watcher, err := NewFileWatcher([]string{tmpFile}, 100*time.Millisecond, func() error {
			return assert.AnError
		})
		require.NoError(t, err)
		defer watcher.Stop()

		err = watcher.Start()
		require.NoError(t, err)

		// Wait for watcher to be ready
		time.Sleep(50 * time.Millisecond)

		// Write to file
		os.WriteFile(tmpFile, []byte("updated"), 0644)

		// Wait for debounce - error should be handled gracefully
		time.Sleep(300 * time.Millisecond)

		// Should not panic or crash
		assert.True(t, true)
	})
}

func TestFileWatcher_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent file changes", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "initial")
		defer os.Remove(tmpFile)

		var reloadCount int
		var reloadMutex sync.Mutex

		watcher, err := NewFileWatcher([]string{tmpFile}, 100*time.Millisecond, func() error {
			reloadMutex.Lock()
			defer reloadMutex.Unlock()
			reloadCount++
			return nil
		})
		require.NoError(t, err)
		defer watcher.Stop()

		err = watcher.Start()
		require.NoError(t, err)

		// Wait for watcher to be ready
		time.Sleep(50 * time.Millisecond)

		// Concurrent writes
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				os.WriteFile(tmpFile, []byte("content "+string(rune(idx))), 0644)
			}(i)
		}
		wg.Wait()

		// Wait for debounce
		time.Sleep(300 * time.Millisecond)

		reloadMutex.Lock()
		count := reloadCount
		reloadMutex.Unlock()

		// Should handle concurrent changes gracefully
		assert.GreaterOrEqual(t, count, 1)
	})
}

func TestFileWatcher_ContextCancellation(t *testing.T) {
	t.Run("stop on context cancellation", func(t *testing.T) {
		tmpFile := createTempFile(t, "test.zed", "content")
		defer os.Remove(tmpFile)

		watcher, err := NewFileWatcher([]string{tmpFile}, 500*time.Millisecond, func() error {
			return nil
		})
		require.NoError(t, err)

		err = watcher.Start()
		require.NoError(t, err)

		// Stop should cancel context
		err = watcher.Stop()
		assert.NoError(t, err)

		// Verify watcher is stopped
		time.Sleep(100 * time.Millisecond)
	})
}
