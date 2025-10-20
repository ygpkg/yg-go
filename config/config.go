package config

import (
	"fmt"
	"io"
	"os"

	"github.com/ygpkg/yg-go/settings/remote"
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
	App           string            `yaml:"app"`
	HttpAddr      string            `yaml:"http_addr"`
	GrpcAddr      string            `yaml:"grpc_addr"`
	OpenDocsAPI   bool              `yaml:"open_docs_api"`
	MysqlConns    map[string]string `yaml:"mysql_conns"`
	PostgresConns map[string]string `yaml:"postgres_conns"`
	Env           string            `yaml:"env"`
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

// LoadCoreConfigFromEnv 通过环境变量获取远程配置
// YGCFG_AK
// YGCFG_SK
// YGCFG_GROUP
// YGCFG_KEY
func LoadCoreConfigFromEnv() (*CoreConfig, error) {
	for _, envKey := range []string{"YGCFG_AK", "YGCFG_SK", "YGCFG_GROUP", "YGCFG_KEY"} {
		if os.Getenv(envKey) == "" {
			return nil, fmt.Errorf("%s is empty", envKey)
		}
	}
	cfg := &CoreConfig{}
	err := remote.GetRemoteYAML(os.Getenv("YGCFG_KEY"), cfg)
	if err != nil {
		return nil, err
	}
	std = cfg
	return cfg, nil
}

// LoadCoreConfig 自动获取配置
func LoadCoreConfig(configPath ...string) (*CoreConfig, error) {
	if len(configPath) > 0 && configPath[0] != "" {
		return LoadCoreConfigFromFile(configPath[0])
	}
	return LoadCoreConfigFromEnv()
}

// LoadYamlLocalFile .
func LoadYamlLocalFile(file string, cfg interface{}) error {
	f, err := os.Open(file)
	if err != nil {
		fmt.Printf("[config] laod %s failed, %s\n", file, err)
		return err
	}
	defer f.Close()

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

// Env 获取环境
func Env() string {
	if cfgEnv := Conf().MainConf.Env; cfgEnv != "" {
		return cfgEnv
	}
	if env := os.Getenv("ENV"); env != "" {
		return env
	}
	return ""
}

// IsProd 是否生产环境
func IsProd() bool {
	return Env() == "prod"
}

// IsDev 是否开发环境
func IsDev() bool {
	return Env() == "dev"
}
