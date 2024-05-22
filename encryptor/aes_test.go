package encryptor

import (
	"testing"
)

func TestAesEncrypt(t *testing.T) {
	key := []byte("nYwlQsGauP5YMQYT")
	for _, v := range []string{
		"a",
		"aa",
		"aaa",
		"我",
		"我我",
		"我我我",
		"hello world",
	} {
		encrypted, err := AesEncrypt([]byte(v), []byte(key))
		if err != nil {
			t.Fatal(err)
		}

		decrypted, err := AesDecrypt(encrypted, []byte(key))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Encrypted: %s -> %s -> %s", v, encrypted, decrypted)
	}
}

func TestAesEncryptToBase64(t *testing.T) {
	key := []byte("nYwlQsGauP5YMQYT")
	for _, v := range []string{
		"a",
		"aa",
		"aaa",
		"我",
		"我我",
		"我我我",
		"hello world",
	} {
		encrypted, err := AesEncryptToBase64([]byte(v), []byte(key))
		if err != nil {
			t.Fatal(err)
		}

		decrypted, err := AesDecryptFromBase64(encrypted, []byte(key))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Encrypted: %s -> %s -> %s", v, encrypted, decrypted)
	}
}

func TestAesEncryptToBase58(t *testing.T) {
	key := []byte("nYwlQsGauP5YMQYT")
	for _, v := range []string{
		"a",
		"aa",
		"aaa",
		"我",
		"我我",
		"我我我",
		"hello world",
	} {
		encrypted, err := AesEncryptToBase58([]byte(v), []byte(key))
		if err != nil {
			t.Fatal(err)
		}

		decrypted, err := AesDecryptFromBase58(encrypted, []byte(key))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Encrypted: %s -> %s -> %s", v, encrypted, decrypted)
	}
}

func TestHmacHash(t *testing.T) {
	key := "mykey"
	message := "hello world"
	str := HmacMD5(key, message)
	t.Log(str)
}
