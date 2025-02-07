// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"

	"github.com/daytonaio/daytona/pkg/models"
	"github.com/daytonaio/daytona/pkg/ssh"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func (d *DockerClient) DestroyTarget(target *models.Target, targetDir string, sshClient *ssh.Client) error {
	return nil
}

func (d *DockerClient) DestroyWorkspace(workspace *models.Workspace, workspaceDir string, sshClient *ssh.Client) error {
	ctx := context.Background()

	containerName := d.GetWorkspaceContainerName(workspace)

	err := d.apiClient.ContainerRemove(ctx, containerName, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil && !client.IsErrNotFound(err) {
		return err
	}

	err = d.apiClient.VolumeRemove(ctx, containerName, true)
	if err != nil && !client.IsErrNotFound(err) {
		return err
	}

	return nil
}
