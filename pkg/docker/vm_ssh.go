// Copyright 2024 Daytona Platforms Inc.
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"golang.org/x/crypto/ssh"
)

func (d *DockerClient) WaitForMacOsBoot(containerID string, hostname *string) error {
	c, err := d.apiClient.ContainerInspect(context.TODO(), containerID)
	if err != nil {
		return err
	}
	port := c.NetworkSettings.Ports["22/tcp"][0].HostPort

	addr := fmt.Sprintf("localhost:%s", port)
	if hostname != nil {
		addr = fmt.Sprintf("%s:%s", *hostname, port)
	}

	config := ssh.ClientConfig{
		User:            "daytona",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
		Auth: []ssh.AuthMethod{
			ssh.Password("daytona"),
		},
	}

	for {
		time.Sleep(time.Second * 1)

		c, err := d.apiClient.ContainerInspect(context.TODO(), containerID)
		if err != nil {
			return err
		}

		if c.State.ExitCode != 0 || c.State.Error != "" {
			return fmt.Errorf("container exited with error: %s", c.State.Error)
		}

		conn, err := ssh.Dial("tcp", addr, &config)
		if err != nil {
			continue
		}
		defer conn.Close()

		break
	}
	return nil
}

func (d *DockerClient) GetSshClient(hostname *string, containerData types.ContainerJSON) (*ssh.Client, error) {
	port := containerData.NetworkSettings.Ports["22/tcp"][0].HostPort
	addr := fmt.Sprintf("localhost:%s", port)
	if hostname != nil {
		addr = fmt.Sprintf("%s:%s", *hostname, port)
	}

	config := ssh.ClientConfig{
		User:            "daytona",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
		Auth: []ssh.AuthMethod{
			ssh.Password("daytona"),
		},
	}
	conn, err := ssh.Dial("tcp", addr, &config)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (d *DockerClient) ExecuteCommand(cmd string, logWriter io.Writer, conn *ssh.Client) error {
	session, err := conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	if logWriter != nil {
		session.Stdout = logWriter
		session.Stderr = logWriter
	}
	return session.Run(cmd)
}
