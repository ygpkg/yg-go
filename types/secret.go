package types

import (
	"encoding/json"
	"math/big"
	"strings"

	"github.com/ygpkg/yg-go/encryptor"
	"github.com/ygpkg/yg-go/logs"
)

// Password 密码
type Password string

var secretKey = []byte("q84n7hz7k4b4tcb0")

const (
	encryptorPrefix = "enc:"
)

type Secret string

// Enc 加密
func (s Secret) Enc() Secret {
	if strings.HasPrefix(string(s), encryptorPrefix) {
		return s
	}
	enc, err := encryptor.BlowfishEncryptToBase58(secretKey, []byte(s))
	if err != nil {
		logs.Errorf("encryptor.BlowfishEncryptToBase58(%s) error: %v", string(s), err)
		return s
	}
	return Secret(encryptorPrefix + enc)
}

// Dec 解密
func (s Secret) Dec() Secret {
	if !strings.HasPrefix(string(s), encryptorPrefix) {
		return s
	}
	enc := strings.TrimPrefix(string(s), encryptorPrefix)
	dec, err := encryptor.BlowfishDecryptFromBase58(secretKey, enc)
	if err != nil {
		logs.Errorf("encryptor.BlowfishDecryptFromBase58(%s) error: %v", enc, err)
		return s
	}
	return Secret(dec)
}

// SafeID 是一个安全的ID
type SafeID uint

// Enc 加密的ID
func (id SafeID) Enc() string {
	encStr, err := enc(big.NewInt(int64(id)).Bytes())
	if err != nil {
		logs.Errorf("enc(%v) error: %v", id, err)
		return ""
	}
	return encStr
}
func (id *SafeID) Dec(idstr string) {
	decStr, err := dec(idstr)
	if err != nil {
		logs.Errorf("dec(%s) error: %v", idstr, err)
		return
	}
	a := new(big.Int)
	decID := a.SetBytes(decStr).Int64()

	*id = SafeID(decID)
}

// MarshalJSON .
func (id SafeID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.Enc())
}

// UnmarshalJSON .
func (id *SafeID) UnmarshalJSON(data []byte) error {
	var idstr string
	err := json.Unmarshal(data, &idstr)
	if err != nil {
		return err
	}
	decStr, err := dec(idstr)
	if err != nil {
		logs.Errorf("dec(%s) error: %v", idstr, err)
		return err
	}
	a := new(big.Int)
	decID := a.SetBytes(decStr).Int64()

	*id = SafeID(decID)
	return nil
}

// enc 加密
func enc(data []byte) (string, error) {
	enc, err := encryptor.BlowfishEncryptToBase58(secretKey, data)
	if err != nil {
		return enc, err
	}
	return enc, nil
}

func dec(str string) ([]byte, error) {
	dec, err := encryptor.BlowfishDecryptFromBase58(secretKey, str)
	if err != nil {
		return nil, err
	}
	return dec, nil
}

// GenerateID 生成ID
func GenerateID() string {
	return encryptor.UUID()
}
