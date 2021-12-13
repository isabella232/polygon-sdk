package clean

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

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
	prefix := args[0]

	files, err := ioutil.ReadDir(".")
	if err != nil {
		panic(err)
	}

	localPath := []string{
		"blockchain",
		"trie",
	}

	for _, file := range files {
		path := file.Name()
		if !file.IsDir() {
			continue
		}
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		fmt.Println(path)

		for _, p := range localPath {
			if err := os.RemoveAll(filepath.Join(path, p)); err != nil {
				c.UI.Warn(err.Error())
			}
		}
	}

	return 0
}
