package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseJwtConfig(t *testing.T) {
	data := []byte(`
secret: "123456"
expire: 2h
`)
	cfg := &JwtConfig{}
	err := yaml.Unmarshal(data, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sec %+v", cfg.Expire.Seconds())
}
