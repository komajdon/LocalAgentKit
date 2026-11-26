package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/diag"
	"agent-gui/internal/history"
	"agent-gui/internal/permission"
	"agent-gui/internal/recorder"
	"agent-gui/internal/store"
	"agent-gui/internal/usage"
	"agent-gui/tools"
)

// App is the Wails-bound facade. All public methods are exposed to the frontend.
type App struct {
	ctx context.Context

	startupErr string // set once during startup

	// convMu serialises read-modify-write of stored conversations. It is
	// taken before mu.
	convMu sync.Mutex

	mu            sync.Mutex
	cfg           config.Config
	activeID      string // conversation the UI is showing
	run           *run   // the single active run, or nil
	runSeq        uint64
	restoreStaged bool

	permissions *permission.Manager // set once during startup
	mcp         *tools.MCPPool
	pending     *pendingPrompt // guarded by mu
	promptSeq   uint64         // guarded by mu

	ledger *usage.Ledger
	models modelCache
	log    *slog.Logger // set before the UI can call in

	recMu  sync.Mutex
	recSes *recorder.Session

	// Seams replaced by tests.
	emit        func(name string, data any)
	logf        func(format string, args ...any)
	providerFor func(config.Config) domain.LLMProvider
}

func NewApp() *App {
	// Without a store this cannot fail; startup reloads persisted grants.
	m, _ := permission.NewManager(nil)
	a := &App{permissions: m, providerFor: newProvider, mcp: tools.NewMCPPool(), ledger: usage.NewLedger(usageStore{}), log: diag.Discard()}
	a.emit = func(name string, data any) { runtime.EventsEmit(a.ctx, name, data) }
	a.logf = func(format string, args ...any) { runtime.LogErrorf(a.ctx, format, args...) }
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := store.Init(store.DataDir()); err != nil {
		// Surface the error to the user immediately rather than failing silently.
		// The event can fire before the UI listens; GetStartupError covers that.
		a.startupErr = "Failed to open data store: " + err.Error()
		runtime.EventsEmit(ctx, "app:fatal", a.startupErr)
	}
	if a.startupErr == "" {
		if err := history.Migrate(filepath.Join(store.DataDir(), "backups")); err != nil {
			// Nothing may be written to data this version cannot handle.
			a.startupErr = "Failed to upgrade the data store: " + err.Error()
			_ = store.Close()
			runtime.EventsEmit(ctx, "app:fatal", a.startupErr)
		}
	}
	a.cfg = config.Load()
	log, err := diag.Open(logDir())
	if err != nil {
		runtime.LogErrorf(ctx, "logging is off: %v", err)
	}
	a.log = log
	a.log.Info("start", "version", Version)
	m, err := permission.NewManager(grantStore{})
	if err != nil {
		runtime.LogErrorf(ctx, "saved approvals could not be read; you will be asked again: %v", err)
	}
	a.permissions = m
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	restoring := a.restoreStaged
	active := a.run
	a.mu.Unlock()
	// Let a run stop and save before the store closes.
	if active != nil {
		a.StopAgent()
		select {
		case <-active.done:
		case <-time.After(shutdownWait):
		}
	}
	// After a restore was staged, keep the archives from before it intact.
	if !restoring && a.startupErr == "" {
		if err := writeAutoBackup(filepath.Join(store.DataDir(), "backups"), a.config().BackupRetention); err != nil {
			runtime.LogErrorf(ctx, "automatic backup failed: %v", err)
		}
	}
	a.mcp.Close()
	_ = store.Close()
}

// GetStartupError reports why the app could not start properly, or "".
func (a *App) GetStartupError() string { return a.startupErr }
