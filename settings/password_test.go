package settings

import "testing"

func TestEncryptPassword(t *testing.T) {
	password := "5yrKIImWU4u86E6qmcw"
	encPass := EncryptPassword(password)
	if encPass == password {
		t.Fatal("is equal")
	}
	t.Logf("%s -> %s", password, encPass)
}

func TestDecryptPassword(t *testing.T) {
	password := "5yrKIImWU4u86E6qmcw"
	encPass := EncryptPassword(password)
	if encPass == password {
		t.Fatal("is equal")
	}
	t.Logf("%s -> %s", password, encPass)

	decPass := DecryptPassword(encPass)
	if decPass != password {
		t.Fatal("is not equal")
	}
	t.Logf("%s -> %s -> %s", password, encPass, decPass)
}
