package encryptor

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"hash"
	"io"
	"os"
)

func MD5(v string) string {
	h := md5.New()
	h.Write([]byte(v))
	return hex.EncodeToString(h.Sum(nil))
}

func HmacMD5(key, str string) string {
	return HmacHash(md5.New, key, str)
}

func HmacHash(h func() hash.Hash, key, str string) string {
	mac := hmac.New(h, []byte(key))
	mac.Write([]byte(str))
	return hex.EncodeToString(mac.Sum(nil))
}

func SHA1(v string) string {
	h := sha1.New()
	h.Write([]byte(v))
	return hex.EncodeToString(h.Sum(nil))
}

func SHA1File(path string) (string, error) {
	h := sha1.New()
	return HashFile(h, path)
}

func HashFile(h hash.Hash, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	_, err = io.Copy(h, f)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// MD5File 计算文件的md5值
func MD5File(path string) (string, error) {
	return HashFile(md5.New(), path)
}
