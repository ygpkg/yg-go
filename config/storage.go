package config

import "time"

type FilePurpose = string

// StorageConfig 对象存储配置
type StorageConfig struct {
	StorageOption `yaml:",inline"`

	Local   *LocalStorageConfig `yaml:"local,omitempty"`
	AliOSS  *AliOSSConfig       `yaml:"alioss,omitempty"`
	UpYun   *UpYunConfig        `yaml:"upyun,omitempty"`
	Tencent *TencentCOSConfig   `yaml:"tencent,omitempty"`
}

// StorageOption 对象存储通用配置选项
type StorageOption struct {
	// Purpose 是文件的用途,按业务分类
	Purpose FilePurpose `yaml:"purpose"`
	// PresignedTimeout 预签名超时时间
	PresignedTimeout time.Duration `yaml:"presigned_timeout"`
}

// LocalStorageConfig 。
type LocalStorageConfig struct {
	Dir string `yaml:"dir"`
	// PublicPrefix 公开访问的前缀
	PublicPrefix string `yaml:"public_prefix"`
}

// AliOSSConfig .
type AliOSSConfig struct {
	AliConfig
	Bucket string `yaml:"bucket"`
}

type AliConfig struct {
	RegionID        string `yaml:"region_id"`
	Endpoint        string `yaml:"endpoint"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
}

// UpYunConfig .
type UpYunConfig struct {
	Bucket   string `yaml:"bucket"`
	Operator string `yaml:"operator"`
	Password string `yaml:"password"`
}

// TencentConfig 腾讯云配置
type TencentConfig struct {
	SecretID  string `yaml:"secret_id"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
	Endpoint  string `yaml:"endpoint"`
}

// TencentCOSConfig 腾讯云对象存储配置
type TencentCOSConfig struct {
	TencentConfig `yaml:",inline"`
	Bucket        string `yaml:"bucket"`
}

// MinossConfig Minoss存储配置
type MinossConfig struct {
	EndPoint        string `yaml:"end_point"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	UseSSL          bool   `yaml:"use_ssl"`
	Bucket          string `yaml:"bucket"`
}
