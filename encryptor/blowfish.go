package encryptor

import (
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/mr-tron/base58"
	"golang.org/x/crypto/blowfish"
)

// BlowfishEncryptToBase58 使用 Blowfish 算法加密字符串
func BlowfishEncryptToBase58(key, plaintext []byte) (string, error) {
	block, err := blowfish.NewCipher(key)
	if err != nil {
		return "", err
	}
	ciphertext := make([]byte, blowfish.BlockSize+len(plaintext))
	iv := ciphertext[:blowfish.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[blowfish.BlockSize:], plaintext)
	comped, err := GzipCompress(ciphertext)
	if err != nil {
		return "", err
	}
	return base58.Encode(comped), nil
}

// BlowfishDecryptFromBase58 使用 Blowfish 算法解密字符串
func BlowfishDecryptFromBase58(key []byte, str string) ([]byte, error) {
	encod, err := base58.Decode(str)
	if err != nil {
		return nil, err
	}
	ciphertext, err := GzipDecompress(encod)
	if err != nil {
		return nil, err
	}

	block, err := blowfish.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < blowfish.BlockSize {
		return nil, err
	}
	iv := ciphertext[:blowfish.BlockSize]
	ciphertext = ciphertext[blowfish.BlockSize:]
	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(ciphertext, ciphertext)
	return ciphertext, nil
}
