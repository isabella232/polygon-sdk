package contracts2

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"

	web3 "github.com/umbracle/go-web3"
)

type Metadata struct {
	Bridge web3.Address
}

func (m *Metadata) WriteFile(path string) error {
	fullPath := filepath.Join(path, "metadata.json")

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(fullPath, data, 0755); err != nil {
		return err
	}
	return nil
}

func (m *Metadata) ReadFile(path string) (bool, error) {
	fullPath := filepath.Join(path, "metadata.json")

	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	data, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return false, nil
	}
	if err := json.Unmarshal(data, m); err != nil {
		return false, nil
	}
	return true, nil
}
