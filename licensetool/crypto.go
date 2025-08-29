package licensetool

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"

	"github.com/google/uuid"
	"github.com/ygpkg/yg-go/logs"
)

// ParsePrivateKey will parse a private key (type string) to rsa.PrivateKey
func ParsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the key")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DER encoded private key: %w", err)
	}
	return key, nil
}

// ParsePublicKey will parse a public key (type string) to rsa.PublicKey
func ParsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the key")
	}
	pub, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return pub, nil
}

// GenerateKeys generates public and private key files
func GenerateKeys(ctx context.Context) ([]byte, []byte, error) {
	var (
		privateKeyPem, publicKeyPem []byte
	)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		logs.ErrorContextf(ctx, "GenerateKeys error: %v", err)
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	privateKeyPem = pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	publicKeyPem = pem.EncodeToMemory(&pem.Block{
		Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})
	return privateKeyPem, publicKeyPem, nil
}

// GenerateLicense creates a signed license
func GenerateLicense(ctx context.Context, license *License) error {
	privateKey, err := ParsePrivateKey(license.PrivateKey)
	if err != nil {
		return err
	}

	meta := &Meta{
		Serial:    license.Serial,
		Subject:   license.Subject,
		Env:       license.Env,
		UID:       license.UID,
		Issuer:    license.Issuer,
		ExpiredAt: *license.ExpiredAt,
		Seed:      uuid.NewString(),
	}
	jsonData, err := json.Marshal(meta)
	if err != nil {
		logs.ErrorContextf(ctx, "GenerateLicense marshal[%v] failed: %v", meta, err)
		return err
	}

	hashed := sha256.Sum256(jsonData)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return err
	}

	encodedSignature := base64.StdEncoding.EncodeToString(signature)
	encodedData := base64.StdEncoding.EncodeToString(jsonData)
	licenseString := fmt.Sprintf("%s.%s", encodedSignature, encodedData)

	license.Meta = *meta
	license.Raw = licenseString

	return nil
}
