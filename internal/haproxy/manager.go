package haproxy

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Manager handles HAProxy configuration lifecycle:
// validation, file rotation, and hitless reload.
type Manager struct {
	mu sync.Mutex

	configDir  string // directory containing haproxy config files
	masterSock string // path to HAProxy master CLI socket

	// For testing: allow overriding the validate command.
	validateCmd func(configPath string) ([]byte, error)
}

// NewManager creates a new HAProxy manager.
// configDir is where haproxy.cfg lives (e.g. /etc/haproxy).
// masterSock is the path to the HAProxy master CLI socket.
func NewManager(configDir, masterSock string) *Manager {
	m := &Manager{
		configDir:  configDir,
		masterSock: masterSock,
	}
	m.validateCmd = m.defaultValidate
	return m
}

func (m *Manager) livePath() string { return filepath.Join(m.configDir, "haproxy.cfg") }
func (m *Manager) newPath() string  { return filepath.Join(m.configDir, "haproxy.cfg.new") }
func (m *Manager) bakPath() string  { return filepath.Join(m.configDir, "haproxy.cfg.bak") }

// Apply renders the config, validates it, rotates the files, and triggers a reload.
func (m *Manager) Apply(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Render config to bytes.
	data, err := Render(cfg)
	if err != nil {
		return fmt.Errorf("rendering config: %w", err)
	}

	// 2. Write candidate file.
	slog.Debug("writing candidate config", "path", m.newPath())
	if err := os.WriteFile(m.newPath(), data, 0644); err != nil {
		return fmt.Errorf("writing candidate config: %w", err)
	}

	// 3. Validate candidate.
	slog.Debug("validating candidate config", "path", m.newPath())
	if output, err := m.validateCmd(m.newPath()); err != nil {
		slog.Error("config validation failed", "path", m.newPath(), "output", string(output), "error", err)
		return fmt.Errorf("validating config: %s: %w", string(output), err)
	}
	slog.Debug("config validation passed")

	// 4. Rotate: live -> bak, new -> live.
	if _, err := os.Stat(m.livePath()); err == nil {
		slog.Debug("backing up live config", "from", m.livePath(), "to", m.bakPath())
		if err := copyFile(m.livePath(), m.bakPath()); err != nil {
			return fmt.Errorf("backing up live config: %w", err)
		}
	}
	if err := os.Rename(m.newPath(), m.livePath()); err != nil {
		return fmt.Errorf("promoting candidate config: %w", err)
	}
	slog.Info("config promoted", "path", m.livePath())

	// 5. Trigger hitless reload via master CLI socket.
	slog.Info("triggering haproxy reload", "socket", m.masterSock)
	if err := m.reload(); err != nil {
		slog.Error("haproxy reload failed", "socket", m.masterSock, "error", err)
		return fmt.Errorf("reloading haproxy: %w", err)
	}
	slog.Info("haproxy reload completed")

	return nil
}

func (m *Manager) defaultValidate(configPath string) ([]byte, error) {
	cmd := exec.Command("haproxy", "-c", "-f", configPath)
	return cmd.CombinedOutput()
}

func (m *Manager) reload() error {
	conn, err := net.DialTimeout("unix", m.masterSock, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to master socket: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return fmt.Errorf("setting deadline: %w", err)
	}

	if _, err := conn.Write([]byte("reload\n")); err != nil {
		return fmt.Errorf("sending reload command: %w", err)
	}

	// Read response to confirm.
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if n > 0 {
		resp := string(buf[:n])
		slog.Debug("haproxy master socket response", "response", resp)
	}
	if err != nil && err != io.EOF {
		return fmt.Errorf("reading reload response: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
