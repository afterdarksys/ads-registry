package runtime

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// Manager handles the lifecycle of containers
type Manager struct {
	cli *client.Client
}

// NewManager creates a new container manager
func NewManager() (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &Manager{cli: cli}, nil
}

// StartContainer will start a created container
func (m *Manager) StartContainer(ctx context.Context, id string) error {
	return m.cli.ContainerStart(ctx, id, container.StartOptions{})
}

// StopContainer will stop a running container
func (m *Manager) StopContainer(ctx context.Context, id string) error {
	return m.cli.ContainerStop(ctx, id, container.StopOptions{})
}

// PauseContainer will pause a running container
func (m *Manager) PauseContainer(ctx context.Context, id string) error {
	return m.cli.ContainerPause(ctx, id)
}

// UnpauseContainer will unpause a paused container
func (m *Manager) UnpauseContainer(ctx context.Context, id string) error {
	return m.cli.ContainerUnpause(ctx, id)
}

// GetClient returns the underlying Docker client for direct API access
func (m *Manager) GetClient() *client.Client {
	return m.cli
}

// Close closes the Docker client connection
func (m *Manager) Close() error {
	if m.cli != nil {
		return m.cli.Close()
	}
	return nil
}
