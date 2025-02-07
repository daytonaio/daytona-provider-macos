// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"io"
	"time"

	"github.com/daytonaio/daytona/pkg/models"
)

func (d *DockerClient) StopWorkspace(workspace *models.Workspace, logWriter io.Writer) error {
	containerName := d.GetWorkspaceContainerName(workspace)
	c, err := d.apiClient.ContainerInspect(context.TODO(), containerName)
	if err != nil {
		return err
	}

	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname, c)
	if err != nil {
		return err
	}
	err = d.ExecuteCommand("sudo shutdown -h now", logWriter, sshClient)
	if err == nil {
		return nil
	}

	time.Sleep(time.Second * 2)

	for i := 0; i < 6; i++ {
		client, err := d.GetSshClient(d.targetOptions.RemoteHostname, c)
		if err != nil {
			return nil
		}
		client.Close()
		time.Sleep(time.Millisecond * 500)
		i++
	}

	err = d.apiClient.ContainerKill(context.TODO(), d.GetWorkspaceContainerName(workspace), "SIGKILL")
	if err != nil {
		return err
	}

	return nil
}
