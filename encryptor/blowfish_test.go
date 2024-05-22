package encryptor

import (
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
		t.Log(key, str, encrypted)

		decrypted, err := BlowfishDecryptFromBase58(key, encrypted)
		if err != nil {
			t.Error(err)
		}
		if string(decrypted) != string(str) {
			t.Error("test error")
		}
		t.Log(key, str, encrypted, decrypted)
	}

}
