package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"central/internal/config"
	containertypes "github.com/docker/docker/api/types/container"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
)

type dockerRuntime struct {
	client *client.Client
}

func newDockerRuntime() (*dockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &dockerRuntime{client: cli}, nil
}

func (r *dockerRuntime) start(ctx context.Context, cfg *config.Config, cs config.ChunkServer) (*process, error) {
	if cs.ContainerImage == "" {
		return nil, fmt.Errorf("container_image must be set for docker runtime (chunk server %s)", cs.ID)
	}

	envMap, err := chunkServerEnvironment(cfg, cs)
	if err != nil {
		return nil, err
	}
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	if err := r.ensureImage(ctx, cs.ContainerImage); err != nil {
		return nil, err
	}

	containerCfg := &containertypes.Config{
		Image: cs.ContainerImage,
		Cmd:   cs.Args,
		Env:   env,
	}
	hostCfg := &containertypes.HostConfig{
		AutoRemove: true,
	}

	resp, err := r.client.ContainerCreate(ctx, containerCfg, hostCfg, nil, nil, cs.ID)
	if err != nil {
		return nil, fmt.Errorf("create docker container: %w", err)
	}

	if err := r.client.ContainerStart(ctx, resp.ID, containertypes.StartOptions{}); err != nil {
		return nil, fmt.Errorf("start docker container: %w", err)
	}

	proc := newProcess(cs)
	proc.setActiveStatus("running")

	go r.watchContainer(proc, resp.ID)

	proc.stopFn = func(stopCtx context.Context) error {
		timeout := int((10 * time.Second).Seconds())
		err := r.client.ContainerStop(stopCtx, resp.ID, containertypes.StopOptions{Timeout: &timeout})
		if err != nil && !errdefs.IsNotFound(err) {
			return err
		}
		select {
		case <-proc.doneCh:
			return nil
		case <-stopCtx.Done():
			return stopCtx.Err()
		case <-time.After(10 * time.Second):
			return errors.New("timeout waiting for docker container to stop")
		}
	}

	return proc, nil
}

func (r *dockerRuntime) watchContainer(proc *process, containerID string) {
	statusCh, errCh := r.client.ContainerWait(context.Background(), containerID, containertypes.WaitConditionNotRunning)
	select {
	case result := <-statusCh:
		if result.Error != nil {
			proc.setFinalStatus("stopped", errors.New(result.Error.Message))
			return
		}
		if result.StatusCode != 0 {
			proc.setFinalStatus("stopped", fmt.Errorf("exit status %d", result.StatusCode))
			return
		}
		proc.setFinalStatus("exited", nil)
	case err := <-errCh:
		if err != nil {
			proc.setFinalStatus("stopped", err)
		}
	}
}

func (r *dockerRuntime) ensureImage(ctx context.Context, image string) error {
	_, _, err := r.client.ImageInspectWithRaw(ctx, image)
	if err == nil {
		return nil
	}
	if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("inspect docker image %s: %w", image, err)
	}

	reader, err := r.client.ImagePull(ctx, image, imagetypes.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull docker image %s: %w", image, err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func (r *dockerRuntime) shutdown() {
	if r.client != nil {
		_ = r.client.Close()
	}
}
