package config

type FilePurpose string

const (
	FilePurposeUnknown FilePurpose = "unknown"
	// FilePurposeAvatar 用户头像
	FilePurposeAvatar FilePurpose = "avatar"
	// FilePurposeFlightVideo 飞行视频
	FilePurposeFlightVideo FilePurpose = "flight_video"

	// FilePurposeOcrForm 用户端健康记录ocr单据
	FilePurposeOcrForm FilePurpose = "ocr_form"
	// FilePurposeForm 用户端健康记录上手上传单据文件
	FilePurposeForm FilePurpose = "form"

	// FilePurposeGeneral 通用的存储
	FilePurposeGeneral FilePurpose = "general"
)

type StorageConfig struct {
	StorageOption `yaml:",inline"`

	Local   *LocalStorageConfig `yaml:"local,omitempty"`
	AliOSS  *AliOSSConfig       `yaml:"alioss,omitempty"`
	UpYun   *UpYunConfig        `yaml:"upyun,omitempty"`
	Tencent *TencentCOSConfig   `yaml:"tencent,omitempty"`
}

type StorageOption struct {
	Purpose FilePurpose `yaml:"purpose"`
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
	Endpoint string `yaml:"endpoint"`
	Bucket   string `yaml:"bucket"`
}

type AliConfig struct {
	RegionID        string `yaml:"region_id"`
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
}

// TencentCOSConfig 腾讯云对象存储配置
type TencentCOSConfig struct {
	TencentConfig `yaml:",inline"`
	Bucket        string `yaml:"bucket"`
}
