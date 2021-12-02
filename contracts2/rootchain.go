package contracts2

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

func GetRootChain() (string, *Metadata) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}
	var contID string
	for _, c := range containers {
		if strings.HasPrefix(c.Image, "ethereum/client-go") {
			contID = c.ID
		}
	}
	if contID == "" {
		panic("container not found")
	}

	containerInspect, err := cli.ContainerInspect(ctx, contID)
	if err != nil {
		panic(err)
	}

	ipAddr := containerInspect.NetworkSettings.IPAddress
	addr := fmt.Sprintf("http://%s:8545", ipAddr)

	metadata := &Metadata{}
	if _, err := metadata.ReadFile("./test-rootchain"); err != nil {
		panic(err)
	}
	return addr, metadata
}
