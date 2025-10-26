package cluster

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"central/internal/config"
)

type Manager struct {
	cfg       *config.Config
	mu        sync.RWMutex
	processes map[string]*process
}

type process struct {
	cfg       config.ChunkServer
	cmd       *exec.Cmd
	startedAt time.Time
	stoppedAt *time.Time
	status    string
	lastError string
	mutex     sync.RWMutex
}

type ProcessInfo struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	StartedAt     time.Time  `json:"started_at"`
	StoppedAt     *time.Time `json:"stopped_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	ListenAddress string     `json:"listen_address"`
	HttpAddress   string     `json:"http_address"`
}

func New(cfg *config.Config) *Manager {
	return &Manager{
		cfg:       cfg,
		processes: make(map[string]*process),
	}
}

func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for _, cs := range m.cfg.ChunkServers {
		if _, exists := m.processes[cs.ID]; exists {
			continue
		}
		proc, err := m.startProcess(ctx, cs)
		if err != nil {
			errs = append(errs, fmt.Errorf("chunk server %s: %w", cs.ID, err))
			continue
		}
		m.processes[cs.ID] = proc
	}
	return errors.Join(errs...)
}

func (m *Manager) startProcess(ctx context.Context, cs config.ChunkServer) (*process, error) {
	cmd := exec.CommandContext(ctx, cs.Executable, cs.Args...)
	cmd.Env = os.Environ()
	for k, v := range m.cfg.Cluster.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range cs.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if cs.ListenAddress != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CHUNK_LISTEN=%s", cs.ListenAddress))
	}
	jsonPayload, yamlPayload, err := buildChunkServerConfigPayload(m.cfg, cs)
	if err != nil {
		return nil, err
	}
	if jsonPayload != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CHUNK_CONFIG_JSON=%s", jsonPayload))
	}
	if yamlPayload != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CHUNK_CONFIG_YAML_B64=%s", yamlPayload))
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	proc := &process{
		cfg:       cs,
		cmd:       cmd,
		startedAt: time.Now(),
		status:    "running",
	}
	go proc.watch()
	return proc, nil
}

func (p *process) watch() {
	err := p.cmd.Wait()
	now := time.Now()
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.stoppedAt = &now
	if err != nil {
		p.status = "stopped"
		p.lastError = err.Error()
	} else {
		p.status = "exited"
	}
}

func (m *Manager) Shutdown() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, proc := range m.processes {
		proc.stop()
	}
}

func (p *process) stop() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGINT)
	time.AfterFunc(5*time.Second, func() {
		if p.cmd.ProcessState == nil {
			_ = p.cmd.Process.Kill()
		}
	})
}

func (m *Manager) Processes() []ProcessInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ProcessInfo, 0, len(m.processes))
	for _, proc := range m.processes {
		proc.mutex.RLock()
		info := ProcessInfo{
			ID:            proc.cfg.ID,
			Status:        proc.status,
			StartedAt:     proc.startedAt,
			ListenAddress: proc.cfg.ListenAddress,
			HttpAddress:   proc.cfg.HttpAddress,
			LastError:     proc.lastError,
		}
		if proc.stoppedAt != nil {
			stopped := *proc.stoppedAt
			info.StoppedAt = &stopped
		}
		proc.mutex.RUnlock()
		out = append(out, info)
	}
	return out
}

func buildChunkServerConfigPayload(cfg *config.Config, cs config.ChunkServer) (jsonPayload string, yamlPayload string, err error) {
	if cfg == nil {
		return "", "", fmt.Errorf("cluster config is nil")
	}

	chunkCfg := defaultChunkServerConfig()
	chunkCfg.applyClusterOverrides(cfg, cs)

	jsonBytes, err := json.Marshal(chunkCfg)
	if err != nil {
		return "", "", fmt.Errorf("marshal chunk config json: %w", err)
	}

	yamlBytes, err := yaml.Marshal(chunkCfg)
	if err != nil {
		return "", "", fmt.Errorf("marshal chunk config yaml: %w", err)
	}

	return string(jsonBytes), base64.StdEncoding.EncodeToString(yamlBytes), nil
}
