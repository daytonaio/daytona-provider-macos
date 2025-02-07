// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/daytonaio/daytona/pkg/models"
	"github.com/daytonaio/daytona/pkg/ports"
	"github.com/daytonaio/daytona/pkg/ssh"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	log "github.com/sirupsen/logrus"
)

func (d *DockerClient) CreateTarget(target *models.Target, targetDir string, logWriter io.Writer, sshClient *ssh.Client) error {
	return nil
}

func (d *DockerClient) CreateWorkspace(opts *CreateWorkspaceOptions) error {
	ctx := context.TODO()

	cr := opts.ContainerRegistries.FindContainerRegistryByImageName("dockurr/macos:latest")
	err := d.PullImage("dockurr/macos:latest", cr, opts.LogWriter)
	if err != nil {
		return err
	}

	mounts := []mount.Mount{}
	var availablePort *uint16
	portBindings := make(map[nat.Port][]nat.PortBinding)
	portBindings["22/tcp"] = []nat.PortBinding{
		{
			HostIP:   "0.0.0.0",
			HostPort: "10022",
		},
	}
	portBindings["2222/tcp"] = []nat.PortBinding{
		{
			HostIP:   "0.0.0.0",
			HostPort: "2222",
		},
	}

	uiPort := 8006
	for {
		if !ports.IsPortAvailable(uint16(uiPort)) {
			uiPort++
			continue
		}
		portBindings["8006/tcp"] = []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: fmt.Sprintf("%d", uiPort),
			},
		}
		break
	}

	if d.IsLocalMacTarget(opts.Workspace.Target.TargetConfig.ProviderInfo.Name, opts.Workspace.Target.TargetConfig.Options, opts.Workspace.Target.TargetConfig.ProviderInfo.RunnerId) {
		p, err := ports.GetAvailableEphemeralPort()
		if err != nil {
			log.Error(err)
		} else {
			availablePort = &p
			portBindings["2280/tcp"] = []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: fmt.Sprintf("%d", *availablePort),
				},
			}
		}
	}

	c, err := d.apiClient.ContainerCreate(ctx, GetContainerCreateConfig(opts.Workspace, availablePort), &container.HostConfig{
		Privileged: true,
		Mounts:     mounts,
		ExtraHosts: []string{
			"host.docker.internal:host-gateway",
		},
		PortBindings: portBindings,
		Resources: container.Resources{
			Devices: []container.DeviceMapping{
				{
					PathOnHost:      "/dev/kvm",
					PathInContainer: "/dev/kvm",
				},
				{
					PathOnHost:      "/dev/net/tun",
					PathInContainer: "/dev/net/tun",
				},
			},
		},
		CapAdd: []string{
			"NET_ADMIN",
			"SYS_ADMIN",
		},
	}, nil, nil, d.GetWorkspaceContainerName(opts.Workspace))
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	err = d.apiClient.ContainerStart(ctx, c.ID, container.StartOptions{})
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	var containerData types.ContainerJSON
	for {
		containerData, err = d.apiClient.ContainerInspect(ctx, c.ID)
		if err != nil {
			return fmt.Errorf("failed to inspect container when creating project: %w", err)
		}

		if containerData.State.Running {
			break
		}

		time.Sleep(1 * time.Second)
	}
	hostName := "localhost"
	if d.targetOptions.RemoteHostname != nil {
		hostName = *d.targetOptions.RemoteHostname
	}

	opts.LogWriter.Write([]byte(fmt.Sprintf("Visit http://%s:%d and Set up MacOS \n", hostName, uiPort)))
	opts.LogWriter.Write([]byte("Set USERNAME \"daytona\" and PASSWORD \"daytona\" \n"))
	opts.LogWriter.Write([]byte("Please turn on Remote Login to continue.....\n"))
	time.Sleep(15 * time.Second)

	d.OpenWebUI(d.targetOptions.RemoteHostname, containerData, opts.LogWriter)

	err = d.WaitForMacOsBoot(c.ID, d.targetOptions.RemoteHostname)
	if err != nil {
		return fmt.Errorf("failed to wait for mac to boot: %w", err)
	}

	sshClient, err := d.GetSshClient(d.targetOptions.RemoteHostname)
	if err != nil {
		return fmt.Errorf("failed to get SSH client: %w", err)
	}
	defer sshClient.Close()

	opts.LogWriter.Write([]byte("Configuring MacOS \n"))

	commands := []string{
		`echo 'daytona' | sudo -S bash -c 'echo "daytona ALL=(ALL) NOPASSWD:ALL" | sudo EDITOR="tee -a" visudo'`,
		`NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)" -y`,
		"echo >> /Users/daytona/.zprofile",
		`echo 'eval "$(/usr/local/bin/brew shellenv)"' >> ~/.zshrc`,
		`source ~/.zshrc`,
		"(curl -sf -L https://download.daytona.io/daytona/install.sh | sudo bash)",
	}

	for _, command := range commands {
		err = d.ExecuteCommand(command, nil, sshClient)
		if err != nil {
			return fmt.Errorf("failed to execute command %s: %w", command, err)
		}
	}

	opts.LogWriter.Write([]byte("Setting up environment variables\n"))

	for key, env := range opts.Workspace.EnvVars {
		err = d.ExecuteCommand(fmt.Sprintf("echo 'export %s=\"%s\"' >> ~/.zshrc", key, env), nil, sshClient)
		if err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)

		}
	}

	err = d.ExecuteCommand("source ~/.zshrc", nil, sshClient)
	if err != nil {
		return fmt.Errorf("failed to run 'source ~/.zshrc': %w", err)
	}

	return nil
}

func GetContainerCreateConfig(workspace *models.Workspace, toolboxApiHostPort *uint16) *container.Config {
	envVars := []string{
		fmt.Sprintf("ARGUMENTS=%s", "-device e1000,netdev=net0  -netdev user,id=net0,hostfwd=tcp::22-:22,hostfwd=tcp::2222-:2222,hostfwd=tcp::2280-:2280"),
	}
	for key, value := range workspace.EnvVars {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	labels := map[string]string{
		"daytona.target.id":                workspace.TargetId,
		"daytona.workspace.id":             workspace.Id,
		"daytona.workspace.repository.url": workspace.Repository.Url,
	}

	if toolboxApiHostPort != nil {
		labels["daytona.toolbox.api.hostPort"] = fmt.Sprintf("%d", *toolboxApiHostPort)
	}

	exposedPorts := nat.PortSet{}
	if toolboxApiHostPort != nil {
		exposedPorts["2280/tcp"] = struct{}{}
	}

	exposedPorts["22/tcp"] = struct{}{}
	exposedPorts["2222/tcp"] = struct{}{}

	return &container.Config{
		Hostname: workspace.Id,
		Image:    "dockurr/macos:latest",
		Labels:   labels,
		User:     "root",
		Entrypoint: []string{
			"/usr/bin/tini",
			"-s",
			"/run/entry.sh",
		},
		Env:          envVars,
		AttachStdout: true,
		AttachStderr: true,
		ExposedPorts: exposedPorts,
		StopTimeout:  &[]int{120}[0],
	}
}
