package encryptor

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// GenerateRSAPairKeyPEM 生成 RSA 密钥对并保存为 PEM 格式
func GenerateRSAPairKeyPEM() (privateKeyPEM, publicKeyPEM []byte, err error) {
	privateKeyBytes, publicKeyBytes, err := GenerateRSAPairKey()
	if err != nil {
		return
	}

	privateKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	publicKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})
	return
}

// GenerateRSAPairKey 生成 RSA 密钥对
func GenerateRSAPairKey() (privateKeyBytes, publicKeyBytes []byte, err error) {
	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return
	}
	// 保存私钥为 PEM 格式
	privateKeyBytes = x509.MarshalPKCS1PrivateKey(privateKey)
	// 生成公钥
	publicKey := &privateKey.PublicKey
	// 保存公钥为 PEM 格式
	publicKeyBytes, err = x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return
	}
	return
}

// RSAPrivateKeyFromPEM 从 PEM 格式的私钥中解析 RSA 私钥
func RSAPrivateKeyFromPEM(privateKeyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// RSAPublicKeyFromPEM 从 PEM 格式的公钥中解析 RSA 公钥
func RSAPublicKeyFromPEM(publicKeyPEM []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(publicKeyPEM)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("failed to decode PEM block containing public key")
	}
	ifc, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return ifc.(*rsa.PublicKey), nil
}

// RSAPrivateKeyFromFile 从文件中读取 RSA 私钥
func RSAPrivateKeyFromFile(filename string) (*rsa.PrivateKey, error) {
	privateKeyBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return RSAPrivateKeyFromPEM(privateKeyBytes)
}

// RSAPublicKeyFromFile 从文件中读取 RSA 公钥
func RSAPublicKeyFromFile(filename string) (*rsa.PublicKey, error) {
	publicKeyBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return RSAPublicKeyFromPEM(publicKeyBytes)
}

// SignRSASimple 使用 RSA 私钥对数据进行签名
func SignRSASimple(privateKey *rsa.PrivateKey, data []byte) ([]byte, error) {
	return rsa.SignPKCS1v15(rand.Reader, privateKey, 0, data)
}

// VerifyRSASimple 使用 RSA 公钥对签名进行验证
func VerifyRSASimple(publicKey *rsa.PublicKey, data, signature []byte) error {
	return rsa.VerifyPKCS1v15(publicKey, 0, data, signature)
}

// SignRSA 使用 RSA 私钥对数据进行签名
func SignRSA(privateKey *rsa.PrivateKey, data []byte) ([]byte, error) {
	hashed := sha256.Sum256(data)
	return rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
}

// VerifyRSA 使用 RSA 公钥对签名进行验证
func VerifyRSA(publicKey *rsa.PublicKey, data, signature []byte) error {
	hashed := sha256.Sum256(data)
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature)
}
