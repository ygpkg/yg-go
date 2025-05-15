package encryptor

import (
	"encoding/hex"
	"testing"
)

func TestBlowfishEncryptString(t *testing.T) {
	key := []byte("1q2w3e4r5t6y7u8i9o0p")
	for _, str := range [][]byte{
		[]byte("hello world"),
		[]byte("hello world 你好 1234567890"),
		[]byte("1"),
		[]byte("1234567890"),
		[]byte("1234567890-1"),
		[]byte("1234567890-1234567890"),
	} {
		encrypted, err := BlowfishEncryptToBase58(key, str)
		if err != nil {
			t.Error(err)
		}
		t.Log(string(key), string(str), encrypted)

		decrypted, err := BlowfishDecryptFromBase58(key, encrypted)
		if err != nil {
			t.Error(err)
		}
		if string(decrypted) != string(str) {
			t.Error("test error")
		}
		t.Log(string(key), string(str), string(encrypted), string(decrypted))
	}

}

func TestBlowfishEncrypt(t *testing.T) {
	key := []byte("1q2w3e4r5t6y7u8i9o0p")
	for _, str := range [][]byte{
		[]byte("hello world"),
		[]byte("hello world 你好 1234567890"),
		[]byte("1"),
		[]byte("1234567890"),
		[]byte("1234567890-1"),
		[]byte("1234567890-1234567890"),
	} {
		encrypted, err := BlowfishEncrypt(key, str)
		if err != nil {
			t.Error(err)
		}

		decrypted, err := BlowfishDecrypt(key, encrypted)
		if err != nil {
			t.Error(err)
		}
		if string(decrypted) != string(str) {
			t.Error("test error")
		}

		t.Log(string(key), string(str), hex.EncodeToString(encrypted), string(decrypted))
	}
}

func TestBlowfishEncryptCBC(t *testing.T) {
	key := []byte("1q2w3e4r5t6y7u8i9o0p")
	for _, str := range [][]byte{
		[]byte("helllllo"),
		[]byte("hello world 你好 1234567890"),
		[]byte("1"),
		[]byte("1234567890"),
		[]byte("1234567890-1"),
		[]byte("1234567890-1234567890"),
	} {
		encrypted, err := EncryptBlowfishCBC(key, str)
		if err != nil {
			t.Error(err)
		}
		// t.Logf("encrypted....: %v", EncodeBase64(encrypted))

		decrypted, err := DecryptBlowfishCBC(key, encrypted)
		if err != nil {
			t.Error(err)
		}
		if string(decrypted) != string(str) {
			t.Error("test error")
		}
		t.Log(string(key), string(str), hex.EncodeToString(encrypted), string(decrypted))
	}
}
