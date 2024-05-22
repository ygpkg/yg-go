package settings

import (
	"strings"

	"github.com/ygpkg/yg-go/encryptor"
	"github.com/ygpkg/yg-go/logs"
)

const (
	passwordAESKey          = "T651qzaEFL6Dpudy"
	passwordEncryptedPrefix = "encryped:"
)

// EncryptPassword 对密码进行加密
func EncryptPassword(oriData string) string {
	if strings.HasPrefix(oriData, passwordEncryptedPrefix) {
		return oriData
	}
	encrypted, err := encryptor.AesEncryptToBase64([]byte(oriData), []byte(passwordAESKey))
	if err != nil {
		logs.Errorf("[settings] encrypt password failed, %s", err)
		return oriData
	}
	return passwordEncryptedPrefix + encrypted
}

// DecryptPassword 对加密的密码进行解密
func DecryptPassword(encData string) string {
	if !strings.HasPrefix(encData, passwordEncryptedPrefix) {
		return encData
	}
	oriData := strings.TrimPrefix(encData, passwordEncryptedPrefix)
	decrypted, err := encryptor.AesDecryptFromBase64(oriData, []byte(passwordAESKey))
	if err != nil {
		logs.Errorf("[settings] decrypt password failed, %s", err)
		return oriData
	}
	return string(decrypted)
}
