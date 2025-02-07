// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
)

func (d *DockerClient) StartWorkspace(opts *CreateWorkspaceOptions, daytonaDownloadUrl string) error {
	containerName := d.GetWorkspaceContainerName(opts.Workspace)
	c, err := d.apiClient.ContainerInspect(context.TODO(), containerName)
	if err != nil {
		return fmt.Errorf("failed to inspect container when starting project: %w", err)
	}

	if !c.State.Running {
		err = d.apiClient.ContainerStart(context.TODO(), containerName, container.StartOptions{})
		if err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}

		c, err := d.apiClient.ContainerInspect(context.TODO(), containerName)
		if err != nil {
			return fmt.Errorf("failed to inspect container when starting project: %w", err)
		}

		if !c.State.Running {
			return fmt.Errorf("failed to start container")
		}

		d.OpenWebUI(d.targetOptions.RemoteHostname, c, opts.LogWriter)

		err = d.WaitForMacOsBoot(c.ID, d.targetOptions.RemoteHostname)
		if err != nil {
			return err
		}
	}

	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return err
	}

	command := `source ~/.zshrc && osascript -e 'do shell script "daytona agent > /Users/daytona/.daytona-agent.log 2>&1 &"'`
	err = d.ExecuteCommand(command, opts.LogWriter, sshClient)
	if err != nil {
		opts.LogWriter.Write([]byte(fmt.Sprintf("failed to execute command %s: %s\n", command, err.Error())))
	}

	opts.LogWriter.Write([]byte("Daytona agent started\n"))

	return nil
}
