package encryptor

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"

	"github.com/mr-tron/base58"
)

func pkcs5Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func pkcs5UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

// AesEncrypt 加密
func AesEncrypt(origData, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	origData = pkcs5Padding(origData, blockSize)
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	crypted := make([]byte, len(origData))
	blockMode.CryptBlocks(crypted, origData)
	return crypted, nil
}

// AesDecrypt 解密
func AesDecrypt(crypted, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)
	origData = pkcs5UnPadding(origData)
	return origData, nil
}

// AesEncryptToBase64 加密并输出base64后的字符串
func AesEncryptToBase64(origData, key []byte) (string, error) {
	encripted, err := AesEncrypt(origData, key)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encripted), nil
}

// AesDecryptFromBase64 解密被base64过的密文
func AesDecryptFromBase64(crypted string, key []byte) ([]byte, error) {
	buf, err := base64.StdEncoding.DecodeString(crypted)
	if err != nil {
		return nil, err
	}
	return AesDecrypt(buf, key)
}

// AesEncryptToBase58 加密并输出base58后的字符串
func AesEncryptToBase58(origData, key []byte) (string, error) {
	encripted, err := AesEncrypt(origData, key)
	if err != nil {
		return "", err
	}
	return base58.Encode(encripted), nil
}

// AesDecryptFromBase58 解密被base58过的密文
func AesDecryptFromBase58(crypted string, key []byte) ([]byte, error) {
	buf, err := base58.Decode(crypted)
	if err != nil {
		return nil, err
	}
	return AesDecrypt(buf, key)
}
