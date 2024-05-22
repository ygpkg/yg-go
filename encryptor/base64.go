package encryptor

import "encoding/base64"

type (
	encryptFunc func(key, plainText []byte) ([]byte, error)
	decryptFunc func(key, cipherText []byte) ([]byte, error)
)

// EncodeBase64 编码为 Base64 字符串
func EncodeBase64(src []byte) string {
	return base64.StdEncoding.EncodeToString(src)
}

// DecodeBase64 解码 Base64 字符串
func DecodeBase64(src string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(src)
}

// EncryptToBase64 加密并输出 Base64 字符串
func EncryptToBase64(ef encryptFunc, key, src []byte) (string, error) {
	encrypted, err := ef(key, src)
	if err != nil {
		return "", err
	}
	return EncodeBase64(encrypted), nil
}

// DecryptFromBase64 解密被 Base64 过的密文
func DecryptFromBase64(df decryptFunc, key []byte, src string) ([]byte, error) {
	buf, err := DecodeBase64(src)
	if err != nil {
		return nil, err
	}
	return df(key, buf)
}
