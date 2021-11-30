package clean

import (
	"os"
	"path/filepath"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/mitchellh/cli"
)

// CleanCommand is the command to show the version of the agent
type CleanCommand struct {
	UI cli.Ui

	helper.Meta
}

// Help implements the cli.Command interface
func (c *CleanCommand) Help() string {
	return ""
}

// Synopsis implements the cli.Command interface
func (c *CleanCommand) Synopsis() string {
	return ""
}

func (c *CleanCommand) GetBaseCommand() string {
	return "clean"
}

// Run implements the cli.Command interface
func (c *CleanCommand) Run(args []string) int {
	path := args[0]

	localPath := []string{
		"blockchain",
		"trie",
	}
	for _, p := range localPath {
		if err := os.RemoveAll(filepath.Join(path, p)); err != nil {
			c.UI.Warn(err.Error())
		}
	}
	return 0
}
