package updater

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Launcher manages the NVR process lifecycle
// It's responsible for starting, stopping, and restarting the main NVR process
type Launcher struct {
	binaryPath string
	args       []string
	workDir    string

	cmd    *exec.Cmd
	mu     sync.Mutex
	logger *slog.Logger

	// Restart control
	restartCh chan struct{}
	stopCh    chan struct{}

	// State
	running     bool
	restartNext bool
}

// NewLauncher creates a new launcher
func NewLauncher(binaryPath string, args []string, logger *slog.Logger) *Launcher {
	return &Launcher{
		binaryPath: binaryPath,
		args:       args,
		workDir:    filepath.Dir(binaryPath),
		logger:     logger,
		restartCh:  make(chan struct{}, 1),
		stopCh:     make(chan struct{}),
	}
}

// Run starts and manages the NVR process
// This should be called from the main entrypoint
func (l *Launcher) Run(ctx context.Context) error {
	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		// Start the process
		if err := l.start(ctx); err != nil {
			l.logger.Error("Failed to start NVR process", "error", err)
			// Wait before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				continue
			}
		}

		l.logger.Info("NVR process started", "pid", l.cmd.Process.Pid)

		// Wait for process to exit or signals
		exitCh := make(chan error, 1)
		go func() {
			exitCh <- l.cmd.Wait()
		}()

		select {
		case <-ctx.Done():
			l.stop()
			return ctx.Err()

		case <-l.stopCh:
			l.stop()
			return nil

		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				// Graceful restart
				l.logger.Info("Received SIGHUP, restarting NVR")
				l.stop()
				continue

			case syscall.SIGINT, syscall.SIGTERM:
				l.logger.Info("Received signal, shutting down", "signal", sig)
				l.stop()
				return nil
			}

		case <-l.restartCh:
			l.logger.Info("Restart requested, restarting NVR")
			l.stop()
			continue

		case err := <-exitCh:
			l.mu.Lock()
			l.running = false
			shouldRestart := l.restartNext
			l.restartNext = false
			l.mu.Unlock()

			if err != nil {
				l.logger.Error("NVR process exited with error", "error", err)
			} else {
				l.logger.Info("NVR process exited normally")
			}

			if shouldRestart {
				l.logger.Info("Restarting after update")
				continue
			}

			// Check if we should auto-restart on crash
			if err != nil {
				l.logger.Info("Restarting after crash in 5 seconds")
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(5 * time.Second):
					continue
				}
			}

			return err
		}
	}
}

// start launches the NVR process
func (l *Launcher) start(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running {
		return fmt.Errorf("process already running")
	}

	l.cmd = exec.CommandContext(ctx, l.binaryPath, l.args...)
	l.cmd.Dir = l.workDir
	l.cmd.Stdout = os.Stdout
	l.cmd.Stderr = os.Stderr
	l.cmd.Stdin = os.Stdin

	// Pass through environment
	l.cmd.Env = os.Environ()

	if err := l.cmd.Start(); err != nil {
		return err
	}

	l.running = true
	return nil
}

// stop gracefully stops the NVR process
func (l *Launcher) stop() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.running || l.cmd == nil || l.cmd.Process == nil {
		return
	}

	// Send SIGTERM first
	l.cmd.Process.Signal(syscall.SIGTERM)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		l.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		l.logger.Info("NVR process stopped gracefully")
	case <-time.After(10 * time.Second):
		l.logger.Warn("NVR process did not stop gracefully, killing")
		l.cmd.Process.Kill()
	}

	l.running = false
}

// Restart requests a restart of the NVR process
func (l *Launcher) Restart() {
	l.mu.Lock()
	l.restartNext = true
	l.mu.Unlock()

	select {
	case l.restartCh <- struct{}{}:
	default:
	}
}

// Stop requests a full shutdown
func (l *Launcher) Stop() {
	close(l.stopCh)
}

// IsRunning returns whether the NVR process is running
func (l *Launcher) IsRunning() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running
}

// GetPID returns the PID of the running process, or 0 if not running
func (l *Launcher) GetPID() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.running && l.cmd != nil && l.cmd.Process != nil {
		return l.cmd.Process.Pid
	}
	return 0
}
