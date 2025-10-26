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

	"central/internal/config"
	"gopkg.in/yaml.v3"
)

type runtimeMode string

const (
	runtimeLocal      runtimeMode = "local"
	runtimeDocker     runtimeMode = "docker"
	runtimeKubernetes runtimeMode = "kubernetes"
)

type Manager struct {
	cfg  *config.Config
	mode runtimeMode

	mu        sync.RWMutex
	processes map[string]*process

	docker *dockerRuntime
	kube   *kubernetesRuntime
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

type process struct {
	cfg config.ChunkServer

	startedAt time.Time
	stoppedAt *time.Time
	status    string
	lastError string

	mu sync.RWMutex

	stopFn      func(context.Context) error
	cancelWatch context.CancelFunc
	doneCh      chan struct{}
	doneOnce    sync.Once
}

func New(cfg *config.Config) (*Manager, error) {
	mode := detectRuntimeMode()
	mgr := &Manager{
		cfg:       cfg,
		mode:      mode,
		processes: make(map[string]*process),
	}

	switch mode {
	case runtimeDocker:
		runtime, err := newDockerRuntime()
		if err != nil {
			return nil, fmt.Errorf("initialise docker runtime: %w", err)
		}
		mgr.docker = runtime
	case runtimeKubernetes:
		runtime, err := newKubernetesRuntime()
		if err != nil {
			return nil, fmt.Errorf("initialise kubernetes runtime: %w", err)
		}
		mgr.kube = runtime
	}

	return mgr, nil
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
	switch m.mode {
	case runtimeDocker:
		if m.docker == nil {
			return nil, fmt.Errorf("docker runtime not initialised")
		}
		return m.docker.start(ctx, m.cfg, cs)
	case runtimeKubernetes:
		if m.kube == nil {
			return nil, fmt.Errorf("kubernetes runtime not initialised")
		}
		return m.kube.start(ctx, m.cfg, cs)
	default:
		return m.startLocalProcess(ctx, cs)
	}
}

func (m *Manager) startLocalProcess(ctx context.Context, cs config.ChunkServer) (*process, error) {
	envMap, err := chunkServerEnvironment(m.cfg, cs)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, cs.Executable, cs.Args...)
	cmd.Env = append([]string{}, os.Environ()...)
	for k, v := range envMap {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	proc := newProcess(cs)
	proc.setActiveStatus("running")

	go func() {
		err := cmd.Wait()
		if err != nil {
			proc.setFinalStatus("stopped", err)
		} else {
			proc.setFinalStatus("exited", nil)
		}
	}()

	proc.stopFn = func(stopCtx context.Context) error {
		if cmd.Process == nil {
			return nil
		}
		if err := cmd.Process.Signal(syscall.SIGINT); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		select {
		case <-proc.doneCh:
			return nil
		case <-time.After(5 * time.Second):
			return cmd.Process.Kill()
		case <-stopCtx.Done():
			return stopCtx.Err()
		}
	}

	return proc, nil
}

func (m *Manager) Shutdown() {
	m.mu.RLock()
	processes := make([]*process, 0, len(m.processes))
	for _, proc := range m.processes {
		processes = append(processes, proc)
	}
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, proc := range processes {
		proc.stop(ctx)
	}

	switch m.mode {
	case runtimeDocker:
		if m.docker != nil {
			m.docker.shutdown()
		}
	case runtimeKubernetes:
		if m.kube != nil {
			m.kube.shutdown()
		}
	}
}

func (m *Manager) Processes() []ProcessInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ProcessInfo, 0, len(m.processes))
	for _, proc := range m.processes {
		out = append(out, proc.info())
	}
	return out
}

func (p *process) stop(ctx context.Context) {
	p.mu.RLock()
	stopFn := p.stopFn
	cancel := p.cancelWatch
	doneCh := p.doneCh
	p.mu.RUnlock()

	if cancel != nil {
		cancel()
	}

	if stopFn != nil {
		_ = stopFn(ctx)
	}

	if doneCh != nil {
		select {
		case <-doneCh:
		case <-ctx.Done():
		}
	}
}

func (p *process) info() ProcessInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	info := ProcessInfo{
		ID:            p.cfg.ID,
		Status:        p.status,
		StartedAt:     p.startedAt,
		ListenAddress: p.cfg.ListenAddress,
		HttpAddress:   p.cfg.HttpAddress,
		LastError:     p.lastError,
	}
	if p.stoppedAt != nil {
		stopped := *p.stoppedAt
		info.StoppedAt = &stopped
	}
	return info
}

func newProcess(cs config.ChunkServer) *process {
	return &process{
		cfg:       cs,
		startedAt: time.Now(),
		status:    "starting",
		doneCh:    make(chan struct{}),
	}
}

func (p *process) setActiveStatus(status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
	if status == "running" || status == "pending" {
		p.stoppedAt = nil
		if status == "running" {
			p.lastError = ""
		}
	}
}

func (p *process) setFinalStatus(status string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
	if err != nil {
		p.lastError = err.Error()
	} else {
		p.lastError = ""
	}
	now := time.Now()
	p.stoppedAt = &now
	p.doneOnce.Do(func() {
		close(p.doneCh)
	})
}

func chunkServerEnvironment(cfg *config.Config, cs config.ChunkServer) (map[string]string, error) {
	env := make(map[string]string)
	for k, v := range cfg.Cluster.Env {
		env[k] = v
	}
	for k, v := range cs.Env {
		env[k] = v
	}
	if cs.ListenAddress != "" {
		env["CHUNK_LISTEN"] = cs.ListenAddress
	}

	jsonPayload, yamlPayload, err := buildChunkServerConfigPayload(cfg, cs)
	if err != nil {
		return nil, err
	}
	if jsonPayload != "" {
		env["CHUNK_CONFIG_JSON"] = jsonPayload
	}
	if yamlPayload != "" {
		env["CHUNK_CONFIG_YAML_B64"] = yamlPayload
	}
	return env, nil
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
