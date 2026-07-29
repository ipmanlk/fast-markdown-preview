package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Build metadata injected via -ldflags at release time.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	portFlag := flag.Int("port", 0, "listening port (0 = OS picks a free port)")
	idleTimeout := flag.Duration("idle-timeout", 5*time.Minute, "shut down after this long with no SSE clients and no updates (0 = disable)")
	noConnTimeout := flag.Duration("connection-timeout", 2*time.Minute, "shut down if no browser connects within this time (0 = disable). catches orphaned servers when the browser never opens")
	dataDir := flag.String("data-dir", defaultDataDir(), "directory for the port file")
	verbose := flag.Bool("verbose", false, "enable debug logging")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetLogLoggerLevel(level)

	// Resolve the listener first so we know the real port before doing
	// anything else. Returning the listener (not just the port) avoids a
	// TOCTOU race where another process could grab the port in between.
	listener, port, err := negotiatePort(*portFlag)
	if err != nil {
		slog.Error("port negotiation failed", "err", err)
		os.Exit(1)
	}

	pf := newPortFile(*dataDir)
	if err := pf.write(port, os.Getpid(), version); err != nil {
		// Non-fatal: the ST plugin can still discover the port via stdout.
		slog.Warn("could not write port file", "err", err)
	}
	defer pf.remove()

	// Announce the port on stdout as the very first line. The ST plugin reads
	// this to learn which port the OS assigned.
	fmt.Printf("PORT=%d\n", port)

	broker := NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer broker.Shutdown()

	renderer, err := NewMarkdownRenderer()
	if err != nil {
		slog.Error("markdown renderer init failed", "err", err)
		os.Exit(1)
	}

	h := &Handler{
		broker:    broker,
		md:        renderer,
		version:   version,
		commit:    commit,
		buildTime: buildTime,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.indexHandler)
	mux.HandleFunc("/stream", h.Stream)
	mux.HandleFunc("/update", h.Update)
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/version", h.Version)

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE connections are long-lived; no write timeout
		IdleTimeout:  60 * time.Second,
	}

	// Idle watchdog: shut the server down when nobody is watching and nothing
	// is being typed. This is the safety net that prevents orphaned servers
	// if ST is force-killed.
	if *idleTimeout > 0 {
		go idleWatchdog(ctx, broker, cancel, *idleTimeout)
	}
	// No-connection watchdog: if no browser ever connects (e.g. the user
	// toggled preview but closed the browser immediately, or the plugin
	// crashed before opening it), shut down so the server isn't orphaned.
	// Once a client connects, this disengages permanently.
	if *noConnTimeout > 0 {
		go noConnectionWatchdog(ctx, broker, cancel, *noConnTimeout, startTime)
	}

	// Signal handling: SIGINT/SIGTERM trigger graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", listener.Addr().String(), "version", version)
		err := srv.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			slog.Error("server error", "err", err)
		}
	case sig := <-sigCh:
		slog.Info("received signal, shutting down", "signal", sig)
	case <-ctx.Done():
		// Idle watchdog or another internal trigger requested shutdown.
	}

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("graceful shutdown failed, forcing close", "err", err)
		_ = srv.Close()
	}

	slog.Info("bye")
}

// idleWatchdog polls every 30s. If there are zero SSE clients and no /update
// activity for the configured timeout, it cancels the context to trigger
// graceful shutdown. Polling (rather than a timer) keeps the logic simple and
// the CPU cost negligible: one tick every 30s.
func idleWatchdog(ctx context.Context, broker *Broker, cancel context.CancelFunc, timeout time.Duration) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if broker.ClientCount() == 0 && broker.TimeSinceLastUpdate() >= timeout {
				slog.Info("idle timeout reached, shutting down", "timeout", timeout)
				cancel()
				return
			}
		}
	}
}

// noConnectionWatchdog fires when no browser connects within the timeout after
// server start. It disengages permanently once a single client connects. This
// catches orphaned servers when the plugin starts the server but the browser
// never opens (user closed tab immediately, plugin crashed before open_browser,
// etc.). Once disengaged, the idleWatchdog takes over normal cleanup.
func noConnectionWatchdog(ctx context.Context, broker *Broker, cancel context.CancelFunc, timeout time.Duration, serverStart time.Time) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if broker.HasEverHadClient() {
				return // normal operation; disengage
			}
			if time.Since(serverStart) >= timeout {
				// Re-check after the time comparison: a client may have
				// connected in the narrow window between the two checks.
				if broker.HasEverHadClient() {
					return
				}
				slog.Info("no browser connected within timeout, shutting down", "timeout", timeout)
				cancel()
				return
			}
		}
	}
}
