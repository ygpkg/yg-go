package config

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

var std *CoreConfig

// Config 。
type CoreConfig struct {
	MainConf MainConfig `yaml:"main"`
	LogsConf LogsConfig `yaml:"logger"`
}

// Conf .
func Conf() *CoreConfig {
	if std == nil {
		fmt.Println("config is nil")
		std = &CoreConfig{}
	}
	return std
}

// MainConfig 住配置
type MainConfig struct {
	App         string            `yaml:"app"`
	HttpAddr    string            `yaml:"http_addr"`
	GrpcAddr    string            `yaml:"grpc_addr"`
	OpenDocsAPI bool              `yaml:"open_docs_api"`
	MysqlConns  map[string]string `yaml:"mysql_conns"`
	Env         string            `yaml:"env"`
}

// LoadCoreConfigFromFile .
func LoadCoreConfigFromFile(filepath string) (*CoreConfig, error) {
	cfg := &CoreConfig{}
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
