package embedspicedb

import (
	"time"

	internalwatch "github.com/akoserwal/embedspicedb/internal/watch"
)

// FileWatcher is a thin re-export of the internal watch implementation.
// It is kept in the root package for backwards compatibility, while the implementation lives in `internal/watch`.
type FileWatcher = internalwatch.FileWatcher

// NewFileWatcher creates a new file watcher.
func NewFileWatcher(files []string, debounce time.Duration, reloadFunc func() error) (*FileWatcher, error) {
	return internalwatch.NewFileWatcher(files, debounce, reloadFunc)
}
