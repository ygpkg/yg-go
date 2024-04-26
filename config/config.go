package config

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadYamlLocalFile .
func LoadYamlLocalFile(file string, cfg interface{}) error {
	f, err := os.Open(file)
	if err != nil {
		fmt.Printf("[config] laod %s failed, %s\n", file, err)
		return err
	}

	err = yaml.NewDecoder(f).Decode(cfg)
	if err != nil {
		fmt.Printf("[config] decode %s failed, %s\n", file, err)
		return err
	}

	return nil
}

// LoadYamlReader .
func LoadYamlReader(r io.Reader, cfg interface{}) error {
	err := yaml.NewDecoder(r).Decode(cfg)
	if err != nil {
		fmt.Printf("[config] decode %T failed, %s\n", r, err)
		return err
	}

	return nil
}
