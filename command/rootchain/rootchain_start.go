package rootchain

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/contracts2"
	"github.com/0xPolygon/polygon-sdk/smart-contract/bindings"
	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/cli"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// RootchainStartCommand is the command to show the version of the agent
type RootchainStartCommand struct {
	UI cli.Ui
	helper.Meta

	logger  hclog.Logger
	client  *client.Client
	id      string
	dataDir string
	ipAddr  string

	closeCh chan struct{}
}

// GetHelperText returns a simple description of the command
func (c *RootchainStartCommand) GetHelperText() string {
	return "Start the rootchain command"
}

// Help implements the cli.Command interface
func (c *RootchainStartCommand) Help() string {
	return ""
}

// Synopsis implements the cli.Command interface
func (c *RootchainStartCommand) Synopsis() string {
	return c.GetHelperText()
}

const (
	imageName = "ethereum/client-go"
	imageTag  = "v1.9.25"
)

func (c *RootchainStartCommand) runRootchain() error {
	c.closeCh = make(chan struct{})

	loggerStd := c.logger.StandardWriter(&hclog.StandardLoggerOptions{})
	ctx := context.Background()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	c.client = cli

	// target directory for the chain
	c.dataDir = "test-rootchain"
	if err := os.MkdirAll(c.dataDir, os.ModePerm); err != nil {
		return err
	}

	// try to pull the image
	reader, err := cli.ImagePull(ctx, "docker.io/"+imageName+":"+imageTag, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	io.Copy(loggerStd, reader)

	// create the client
	args := []string{"--dev"}

	// periodic mining
	// args = append(args, "--dev.period", strconv.Itoa(2))

	// add data dir
	args = append(args, "--datadir", "/eth1data")

	// add ipcpath
	args = append(args, "--ipcpath", "/eth1data/geth.ipc")

	// enable rpc
	args = append(args, "--http", "--http.addr", "0.0.0.0", "--http.api", "eth,net,web3,debug")

	// enable ws
	args = append(args, "--ws", "--ws.addr", "0.0.0.0")

	config := &container.Config{
		Image: imageName + ":" + imageTag,
		Cmd:   args,
	}
	hostConfig := &container.HostConfig{
		Binds: []string{
			c.dataDir + ":/eth1data",
		},
	}
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		return err
	}

	// start the client
	if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}
	c.id = resp.ID
	c.logger.Info("Container started", "id", resp.ID)

	containerInspect, err := cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return err
	}
	c.ipAddr = containerInspect.NetworkSettings.IPAddress

	// wait for it to finish
	go func() {
		statusCh, errCh := c.client.ContainerWait(context.Background(), c.id, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			c.UI.Error(err.Error())
		case status := <-statusCh:
			c.UI.Output(fmt.Sprintf("Done with status %d", status.StatusCode))
		}
		close(c.closeCh)
	}()

	return nil
}

var (
	addr0 = web3.Address{0x1}
	addr1 = web3.Address{0x2}
)

func (c *RootchainStartCommand) initialDeploy() error {
	// read metadata file
	var metadata *contracts2.Metadata
	found, err := metadata.ReadFile(c.dataDir)
	if err != nil {
		return err
	}
	if found {
		return nil
	}

	addr := fmt.Sprintf("http://%s:8545", c.ipAddr)

	provider, err := jsonrpc.NewClient(addr)
	if err != nil {
		return err
	}

	time.Sleep(5 * time.Second)

	// deploy bridge
	owner, err := provider.Eth().Accounts()
	if err != nil {
		return err
	}

	txn := bindings.DeployBridge(provider, owner[0], "abcd")
	if err := txn.DoAndWait(); err != nil {
		panic(err)
	}

	metadata = &contracts2.Metadata{
		Bridge: txn.Receipt().ContractAddress,
	}
	if err := metadata.WriteFile(c.dataDir); err != nil {
		return err
	}
	return nil
}

func (c *RootchainStartCommand) gatherLogs() {
	ctx := context.Background()

	opts := types.ContainerLogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Timestamps: false,
		Follow:     true,
		Tail:       "40",
	}
	out, err := c.client.ContainerLogs(ctx, c.id, opts)
	if err != nil {
		panic(err)
	}
	if _, err := io.Copy(os.Stdout, out); err != nil {
		panic(err)
	}
	fmt.Println("logs done")
}

// Run implements the cli.Command interface
func (c *RootchainStartCommand) Run(args []string) int {
	c.logger = hclog.New(&hclog.LoggerOptions{})

	// start the client
	if err := c.runRootchain(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	// gather the logs
	go c.gatherLogs()

	// perform any initial deploy
	if err := c.initialDeploy(); err != nil {
		panic(err) // this should work always so just panic and we will recover
	}

	return c.handleSignals()
}

func (c *RootchainStartCommand) handleSignals() int {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	stop := true

	select {
	case sig := <-signalCh:
		c.UI.Output(fmt.Sprintf("Caught signal: %v", sig))
		c.UI.Output("Gracefully shutting down agent...")
	case <-c.closeCh:
		stop = false
	}

	// close the container if possible
	if stop {
		if err := c.client.ContainerStop(context.Background(), c.id, nil); err != nil {
			c.UI.Error(fmt.Sprintf("Failed to stop container: %v", err))
		}
	}
	return 0
}
