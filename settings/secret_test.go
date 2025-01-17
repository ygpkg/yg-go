package settings

import "testing"

func TestEncryptSecret(t *testing.T) {
	password := "5yrKIImWU4u86E6qmcw"
	encPass := EncryptSecret(password)
	if encPass == password {
		t.Fatal("is equal")
	}
	t.Logf("%s -> %s", password, encPass)
}

func TestDecryptSecret(t *testing.T) {
	password := "5yrKIImWU4u86E6qmcw"
	encPass := EncryptSecret(password)
	if encPass == password {
		t.Fatal("is equal")
	}
	t.Logf("%s -> %s", password, encPass)

	decPass := DecryptSecret(encPass)
	if decPass != password {
		t.Fatal("is not equal")
	}
	t.Logf("%s -> %s -> %s", password, encPass, decPass)
}
