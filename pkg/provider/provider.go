package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"

	internal "github.com/daytonaio/daytona-provider-macos/internal"
	log_writers "github.com/daytonaio/daytona-provider-macos/internal/log"
	"github.com/daytonaio/daytona-provider-macos/pkg/client"
	"github.com/daytonaio/daytona-provider-macos/pkg/types"

	"github.com/daytonaio/daytona-provider-macos/pkg/docker"
	"github.com/daytonaio/daytona/pkg/logs"
	"github.com/daytonaio/daytona/pkg/models"
	"github.com/daytonaio/daytona/pkg/provider"
	provider_util "github.com/daytonaio/daytona/pkg/provider/util"
	"github.com/daytonaio/daytona/pkg/ssh"
	docker_sdk "github.com/docker/docker/client"
)

type MacProvider struct {
	BasePath           *string
	DaytonaDownloadUrl *string
	DaytonaVersion     *string
	ServerUrl          *string
	ApiUrl             *string
	ApiKey             *string
	TargetLogsDir      *string
	WorkspaceLogsDir   *string
	ApiPort            *uint32
	ServerPort         *uint32
	RemoteSockDir      string
}

func (p *MacProvider) Initialize(req provider.InitializeProviderRequest) (*provider_util.Empty, error) {
	tmpDir := "/tmp"
	if runtime.GOOS == "mac" {
		tmpDir = os.TempDir()
		if tmpDir == "" {
			return new(provider_util.Empty), errors.New("could not determine temp dir")
		}
	}

	p.RemoteSockDir = path.Join(tmpDir, "target-socks")

	// Clear old sockets
	err := os.RemoveAll(p.RemoteSockDir)
	if err != nil {
		return new(provider_util.Empty), err
	}
	err = os.MkdirAll(p.RemoteSockDir, 0755)
	if err != nil {
		return new(provider_util.Empty), err
	}

	p.BasePath = &req.BasePath
	p.DaytonaDownloadUrl = &req.DaytonaDownloadUrl
	p.DaytonaVersion = &req.DaytonaVersion
	p.ServerUrl = &req.ServerUrl
	p.ApiUrl = &req.ApiUrl
	p.ApiKey = req.ApiKey
	p.TargetLogsDir = &req.TargetLogsDir
	p.WorkspaceLogsDir = &req.WorkspaceLogsDir
	p.ApiPort = &req.ApiPort
	p.ServerPort = &req.ServerPort

	return new(provider_util.Empty), nil
}

func (p MacProvider) GetInfo() (models.ProviderInfo, error) {
	label := "MacOS"

	return models.ProviderInfo{
		Name:                 "macos-provider",
		Label:                &label,
		AgentlessTarget:      false,
		Version:              internal.Version,
		TargetConfigManifest: *types.GetTargetConfigManifest(),
	}, nil
}

func (p MacProvider) GetPresetTargetConfigs() (*[]provider.TargetConfig, error) {
	return &[]provider.TargetConfig{
		{
			Name:    "local",
			Options: "{\n\t\"Sock Path\": \"/var/run/docker.sock\"\n}",
		},
	}, nil
}

func (p MacProvider) StartTarget(targetReq *provider.TargetRequest) (*provider_util.Empty, error) {
	return new(provider_util.Empty), nil
}

func (p MacProvider) StopTarget(targetReq *provider.TargetRequest) (*provider_util.Empty, error) {
	return new(provider_util.Empty), nil
}

func (p MacProvider) DestroyTarget(targetReq *provider.TargetRequest) (*provider_util.Empty, error) {
	return new(provider_util.Empty), nil
}

func (p MacProvider) DestroyWorkspace(workspaceReq *provider.WorkspaceRequest) (*provider_util.Empty, error) {
	dockerClient, err := p.getClient(workspaceReq.Workspace.Target.TargetConfig.Options)
	if err != nil {
		return new(provider_util.Empty), err
	}

	workspaceDir, err := p.getWorkspaceDir(workspaceReq)
	if err != nil {
		return new(provider_util.Empty), err
	}

	sshClient, err := p.getSshClient(workspaceReq.Workspace.Target.TargetConfig.Options)
	if err != nil {
		return new(provider_util.Empty), err
	}
	if sshClient != nil {
		defer sshClient.Close()
	}

	err = dockerClient.DestroyWorkspace(workspaceReq.Workspace, workspaceDir, sshClient)
	if err != nil {
		return new(provider_util.Empty), err
	}

	return new(provider_util.Empty), nil
}

func (p MacProvider) GetTargetProviderMetadata(targetReq *provider.TargetRequest) (string, error) {
	dockerClient, err := p.getClient(targetReq.Target.TargetConfig.Options)
	if err != nil {
		return "", err
	}

	return dockerClient.GetTargetProviderMetadata(targetReq.Target)
}

func (p MacProvider) StartWorkspace(workspaceReq *provider.WorkspaceRequest) (*provider_util.Empty, error) {
	dockerClient, err := p.getClient(workspaceReq.Workspace.Target.TargetConfig.Options)
	if err != nil {
		return new(provider_util.Empty), err
	}

	logWriter := io.MultiWriter(&log_writers.InfoLogWriter{})
	if p.WorkspaceLogsDir != nil {
		loggerFactory := logs.NewLoggerFactory(logs.LoggerFactoryConfig{
			LogsDir:     *p.WorkspaceLogsDir,
			ApiUrl:      p.ApiUrl,
			ApiKey:      p.ApiKey,
			ApiBasePath: &logs.ApiBasePathWorkspace,
		})
		workspaceLogWriter, err := loggerFactory.CreateLogger(workspaceReq.Workspace.Id, workspaceReq.Workspace.Name, logs.LogSourceProvider)
		if err != nil {
			return new(provider_util.Empty), err
		}
		logWriter = io.MultiWriter(&log_writers.InfoLogWriter{}, workspaceLogWriter)
		defer workspaceLogWriter.Close()
	}

	workspaceDir, err := p.getWorkspaceDir(workspaceReq)
	if err != nil {
		return new(provider_util.Empty), err
	}

	err = dockerClient.StartWorkspace(&docker.CreateWorkspaceOptions{
		Workspace:           workspaceReq.Workspace,
		WorkspaceDir:        workspaceDir,
		ContainerRegistries: workspaceReq.ContainerRegistries,
		BuilderImage:        workspaceReq.BuilderImage,
		LogWriter:           logWriter,
		Gpc:                 workspaceReq.GitProviderConfig,
		SshClient:           nil,
	}, "")
	if err != nil {
		return new(provider_util.Empty), err
	}

	return new(provider_util.Empty), nil
}

func (p MacProvider) StopWorkspace(workspaceReq *provider.WorkspaceRequest) (*provider_util.Empty, error) {
	dockerClient, err := p.getClient(workspaceReq.Workspace.Target.TargetConfig.Options)
	if err != nil {
		return new(provider_util.Empty), err
	}

	logWriter := io.MultiWriter(&log_writers.InfoLogWriter{})
	if p.WorkspaceLogsDir != nil {
		loggerFactory := logs.NewLoggerFactory(logs.LoggerFactoryConfig{
			LogsDir:     *p.WorkspaceLogsDir,
			ApiUrl:      p.ApiUrl,
			ApiKey:      p.ApiKey,
			ApiBasePath: &logs.ApiBasePathWorkspace,
		})
		workspaceLogWriter, err := loggerFactory.CreateLogger(workspaceReq.Workspace.Id, workspaceReq.Workspace.Name, logs.LogSourceProvider)
		if err != nil {
			return new(provider_util.Empty), err
		}
		logWriter = io.MultiWriter(&log_writers.InfoLogWriter{}, workspaceLogWriter)
		defer workspaceLogWriter.Close()
	}

	err = dockerClient.StopWorkspace(workspaceReq.Workspace, logWriter)
	if err != nil {
		return new(provider_util.Empty), err
	}

	return new(provider_util.Empty), nil
}

func (p MacProvider) GetWorkspaceProviderMetadata(workspaceReq *provider.WorkspaceRequest) (string, error) {
	dockerClient, err := p.getClient(workspaceReq.Workspace.Target.TargetConfig.Options)
	if err != nil {
		return "", err
	}

	return dockerClient.GetWorkspaceProviderMetadata(workspaceReq.Workspace)
}

func (p MacProvider) getClient(targetOptionsJson string) (docker.IDockerClient, error) {
	targetOptions, _, err := types.ParseTargetConfigOptions(targetOptionsJson)
	if err != nil {
		return nil, err
	}

	client, err := client.GetClient(*targetOptions, p.RemoteSockDir)
	if err != nil {
		return nil, err
	}

	return docker.NewDockerClient(docker.DockerClientConfig{
		ApiClient:     client,
		TargetOptions: *targetOptions,
	}), nil
}

func (p MacProvider) CheckRequirements() (*[]provider.RequirementStatus, error) {
	var results []provider.RequirementStatus
	ctx := context.Background()

	cli, err := docker_sdk.NewClientWithOpts(docker_sdk.FromEnv, docker_sdk.WithAPIVersionNegotiation())
	if err != nil {
		results = append(results, provider.RequirementStatus{
			Name:   "Docker installed",
			Met:    false,
			Reason: "Docker is not installed",
		})
		return &results, nil
	} else {
		results = append(results, provider.RequirementStatus{
			Name:   "Docker installed",
			Met:    true,
			Reason: "Docker is installed",
		})
	}

	// Check if Docker is running by fetching Docker info
	_, err = cli.Info(ctx)
	if err != nil {
		results = append(results, provider.RequirementStatus{
			Name:   "Docker running",
			Met:    false,
			Reason: "Docker is not running. Error: " + err.Error(),
		})
	} else {
		results = append(results, provider.RequirementStatus{
			Name:   "Docker running",
			Met:    true,
			Reason: "Docker is running",
		})
	}
	return &results, nil
}

func (p *MacProvider) getWorkspaceDir(workspaceReq *provider.WorkspaceRequest) (string, error) {
	return fmt.Sprintf("/Users/daytona/Desktop/%s/%s", workspaceReq.Workspace.Target.Name, workspaceReq.Workspace.Name), nil
}

func (p *MacProvider) getTargetDir(targetReq *provider.TargetRequest) (string, error) {
	return fmt.Sprintf("/Users/daytona/Desktop/%s", targetReq.Target.Name), nil
}

func (p *MacProvider) getSshClient(targetOptionsJson string) (*ssh.Client, error) {
	targetOptions, isLocal, err := types.ParseTargetConfigOptions(targetOptionsJson)
	if err != nil {
		return nil, err
	}

	if isLocal {
		return nil, nil
	}

	return ssh.NewClient(&ssh.SessionConfig{
		Hostname:       *targetOptions.RemoteHostname,
		Port:           *targetOptions.RemotePort,
		Username:       *targetOptions.RemoteUser,
		Password:       targetOptions.RemotePassword,
		PrivateKeyPath: targetOptions.RemotePrivateKey,
	})
}
