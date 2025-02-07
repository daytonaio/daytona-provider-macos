package main

import (
	"os"

	p "github.com/daytonaio/daytona-provider-macos/pkg/provider"
	"github.com/daytonaio/daytona/pkg/provider"
	"github.com/daytonaio/daytona/pkg/runner/providermanager"
	"github.com/hashicorp/go-hclog"
	hc_plugin "github.com/hashicorp/go-plugin"
)

func main() {
	logger := hclog.New(&hclog.LoggerOptions{
		Level:      hclog.Trace,
		Output:     os.Stderr,
		JSONFormat: true,
	})
	hc_plugin.Serve(&hc_plugin.ServeConfig{
		HandshakeConfig: providermanager.ProviderHandshakeConfig,
		Plugins: map[string]hc_plugin.Plugin{
			"macos-provider": &provider.ProviderPlugin{Impl: &p.MacProvider{}},
		},
		Logger: logger,
	})
}
