package config

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

var std *MainConfig

// Conf .
func Conf() *MainConfig {
	if std == nil {
		fmt.Println("config is nil")
		std = &MainConfig{}
	}
	return std
}

// MainConfig 住配置
type MainConfig struct {
	App         string            `yaml:"app"`
	HttpAddr    string            `yaml:"http_addr"`
	GrpcAddr    string            `yaml:"grpc_addr"`
	OpenDocsAPI bool              `yaml:"open_docs_api"`
	BaseURL     string            `yaml:"base_url"`
	WebDir      string            `yaml:"web_dir"`
	MysqlConns  map[string]string `yaml:"mysql_conns"`
	TmpDir      string            `yaml:"tmp_dir"`
	Env         string            `yaml:"env"`
}

// LoadMainConfigFromFile .
func LoadMainConfigFromFile(filepath string) (*MainConfig, error) {
	cfg := &MainConfig{}
	err := LoadYamlLocalFile(filepath, cfg)
	if err != nil {
		return nil, err
	}
	std = cfg
	return cfg, nil
}

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
