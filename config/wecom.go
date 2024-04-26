package config

import (
	"encoding/base64"
)

type WecomConfig struct {
	Apps WecomApps `yaml:"apps"`
}

// WecomApp 企业微信应用
type WecomApp struct {
	Name           string `json:"name" yaml:"name"`
	CompanyID      string `json:"company_id" yaml:"company_id"`
	AgentID        int64  `json:"agent_id" yaml:"agent_id"`
	Secret         string `json:"secret" yaml:"secret"`
	Token          string `json:"token" yaml:"token"`
	EncodingAESKey string `json:"encoding_aes_key" yaml:"encoding_aes_key"`
}

// WecomApps 企业微信应用列表
type WecomApps []WecomApp

// WecomApp 企业微信应用
func (c WecomConfig) WecomApp(name string) WecomApp {
	for _, app := range c.Apps {
		if app.Name == name {
			return app
		}
	}
	return WecomApp{}
}

// IsValide 是否有效
func (c WecomApp) IsValide() bool {
	return c.CompanyID != "" && c.AgentID != 0 && c.Secret != "" && c.Token != "" && c.EncodingAESKey != ""
}

func (cs WecomApp) AESKey() []byte {
	key, err := base64.StdEncoding.DecodeString(cs.EncodingAESKey + "=")
	if err != nil {
		return []byte{}
	}
	return key
}
