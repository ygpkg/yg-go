package config

import "time"

// JwtConfig jwt config
type JwtConfig struct {
	// Secret jwt secret
	Secret string `yaml:"secret"`
	// Expire jwt expire time
	Expire time.Duration `yaml:"expire"`
}
