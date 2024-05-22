package encryptor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/mr-tron/base58"
)

// AesEncrypt 加密
func AesEncrypt(key, plainText []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	plainText = pad(plainText, blockSize)
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	cipherText := make([]byte, len(plainText))
	blockMode.CryptBlocks(cipherText, plainText)
	return cipherText, nil
}

// AesDecrypt 解密
func AesDecrypt(key, cipherText []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	plainText := make([]byte, len(cipherText))
	blockMode.CryptBlocks(plainText, cipherText)
	plainText = unpad(plainText)
	return plainText, nil
}

// AesEncryptToBase64 加密并输出base64后的字符串
func AesEncryptToBase64(key, plainText []byte) (string, error) {
	return EncryptToBase64(AesEncrypt, key, plainText)
}

// AesDecryptFromBase64 解密被base64过的密文
func AesDecryptFromBase64(key []byte, cipherText string) ([]byte, error) {
	return DecryptFromBase64(AesDecrypt, key, cipherText)
}

// AesEncryptToBase58 加密并输出base58后的字符串
func AesEncryptToBase58(key, plainText []byte) (string, error) {
	encripted, err := AesEncrypt(key, plainText)
	if err != nil {
		return "", err
	}
	return base58.Encode(encripted), nil
}

// AesDecryptFromBase58 解密被base58过的密文
func AesDecryptFromBase58(key []byte, cipherText string) ([]byte, error) {
	buf, err := base58.Decode(cipherText)
	if err != nil {
		return nil, err
	}
	return AesDecrypt(key, buf)
}

func EncryptAesCBC(key, plainText []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	paddedText := pad(plainText, aes.BlockSize)
	cipherText := make([]byte, len(paddedText))
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(cipherText, paddedText)

	return append(iv, cipherText...), nil
}

func DecryptAesCBC(key, cipherText []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(cipherText) < aes.BlockSize {
		return nil, fmt.Errorf("cipherText too short")
	}

	iv := cipherText[:aes.BlockSize]
	cipherText = cipherText[aes.BlockSize:]

	plainText := make([]byte, len(cipherText))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plainText, cipherText)

	return unpad(plainText), nil
}
