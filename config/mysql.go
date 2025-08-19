package config

import "fmt"

// MysqlConfig 数据库连接配置结构体
type MysqlConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	Charset  string `yaml:"charset"`
}

// BuildDNS 方法用于生成 DSN (Data Source Name)
func (cfg *MysqlConfig) BuildDNS() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=true&loc=Local", cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database, cfg.Charset)
}
