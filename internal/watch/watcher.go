package watch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	log "github.com/akoserwal/embedspicedb/internal/logging"
)

// FileWatcher watches schema files for changes and triggers reloads.
type FileWatcher struct {
	watcher    *fsnotify.Watcher
	files      []string
	absFiles   map[string]struct{}
	reloadFunc func() error
	debounce   time.Duration
	mu         sync.Mutex
	pending    map[string]time.Time
	timer      *time.Timer
	stopped    bool
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewFileWatcher creates a new file watcher.
func NewFileWatcher(files []string, debounce time.Duration, reloadFunc func() error) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	absFiles := make(map[string]struct{}, len(files))
	for _, file := range files {
		absPath, err := filepath.Abs(file)
		if err != nil {
			cancel()
			_ = watcher.Close()
			return nil, fmt.Errorf("failed to get absolute path for %s: %w", file, err)
		}
		absFiles[absPath] = struct{}{}
	}

	fw := &FileWatcher{
		watcher:    watcher,
		files:      files,
		absFiles:   absFiles,
		reloadFunc: reloadFunc,
		debounce:   debounce,
		pending:    make(map[string]time.Time),
		ctx:        ctx,
		cancel:     cancel,
	}

	return fw, nil
}

// Start begins watching files for changes.
func (fw *FileWatcher) Start() error {
	// Prefer watching the file path directly (much cheaper on kqueue/macOS than watching a large directory).
	// Fall back to watching the parent directory if the file does not exist yet.
	for absPath := range fw.absFiles {
		if _, err := os.Stat(absPath); err == nil {
			if err := fw.watcher.Add(absPath); err != nil {
				return fmt.Errorf("failed to watch file %s: %w", absPath, err)
			}
			log.Ctx(fw.ctx).Debug().Str("file", absPath).Msg("watching schema file for changes")
			continue
		}

		// File doesn't exist; watch the directory so we can detect when it is created.
		dir := filepath.Dir(absPath)
		if err := fw.watcher.Add(dir); err != nil {
			return fmt.Errorf("failed to watch directory %s: %w", dir, err)
		}
		log.Ctx(fw.ctx).Debug().Str("dir", dir).Msg("watching directory for schema changes (file missing at startup)")
	}

	fw.wg.Add(1)
	go fw.watchLoop()

	return nil
}

// Stop stops watching files.
func (fw *FileWatcher) Stop() error {
	fw.mu.Lock()
	fw.stopped = true
	if fw.timer != nil {
		fw.timer.Stop()
		fw.timer = nil
	}
	fw.pending = make(map[string]time.Time)
	fw.mu.Unlock()

	fw.cancel()
	err := fw.watcher.Close()
	fw.wg.Wait()
	return err
}

func (fw *FileWatcher) watchLoop() {
	defer fw.wg.Done()

	for {
		select {
		case <-fw.ctx.Done():
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			// Check if the event is for one of our watched files
			if fw.isWatchedFile(event.Name) {
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					fw.handleFileChange(event.Name)
				}
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Ctx(fw.ctx).Warn().Err(err).Msg("file watcher error")
		}
	}
}

func (fw *FileWatcher) isWatchedFile(path string) bool {
	_, ok := fw.absFiles[path]
	return ok
}

func (fw *FileWatcher) handleFileChange(filePath string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.stopped {
		return
	}

	now := time.Now()
	fw.pending[filePath] = now

	// Cancel existing timer if any
	if fw.timer != nil {
		fw.timer.Stop()
	}

	// Set up debounced reload
	fw.timer = time.AfterFunc(fw.debounce, func() {
		select {
		case <-fw.ctx.Done():
			return
		default:
		}

		fw.mu.Lock()
		defer fw.mu.Unlock()

		if fw.stopped {
			return
		}

		// Check if there are still pending changes
		if len(fw.pending) == 0 {
			return
		}

		log.Ctx(fw.ctx).Info().Strs("files", fw.getPendingFiles()).Msg("schema files changed, reloading")

		// Clear pending
		fw.pending = make(map[string]time.Time)

		// Trigger reload
		if err := fw.reloadFunc(); err != nil {
			log.Ctx(fw.ctx).Error().Err(err).Msg("failed to reload schema")
		}
	})
}

func (fw *FileWatcher) getPendingFiles() []string {
	files := make([]string, 0, len(fw.pending))
	for file := range fw.pending {
		files = append(files, file)
	}
	return files
}
