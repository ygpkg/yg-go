package settings

import (
	"os"
	"strings"

	"github.com/ygpkg/yg-go/encryptor"
	"github.com/ygpkg/yg-go/logs"
)

const (
	secretEncryptedPrefix = "encryped:"
)

var (
	secretAESKey = "T651qzaEFL6Dpudy"
)

func init() {
	skey := os.Getenv("YG_SETTINGS_SECRET_AES_KEY")
	if skey != "" {
		SetSecretAESKey(skey)
	}
}

// SetSecretAESKey 设置密码加密的key
func SetSecretAESKey(key string) {
	secretAESKey = key
}

// EncryptSecret 对密码进行加密
func EncryptSecret(oriData string) string {
	if strings.HasPrefix(oriData, secretEncryptedPrefix) {
		return oriData
	}
	encrypted, err := encryptor.AesEncryptToBase64([]byte(secretAESKey), []byte(oriData))
	if err != nil {
		logs.Errorf("[settings] encrypt secret failed, %s", err)
		return oriData
	}
	return secretEncryptedPrefix + encrypted
}

// DecryptSecret 对加密的密码进行解密
func DecryptSecret(encData string) string {
	if !strings.HasPrefix(encData, secretEncryptedPrefix) {
		return encData
	}
	oriData := strings.TrimPrefix(encData, secretEncryptedPrefix)
	decrypted, err := encryptor.AesDecryptFromBase64([]byte(secretAESKey), oriData)
	if err != nil {
		logs.Errorf("[settings] decrypt secret failed, %s", err)
		return oriData
	}
	return string(decrypted)
}
