package types

import (
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
)

func TestSafeID(t *testing.T) {
	type SA struct {
		UserID    SafeID     `json:"user_id"`
		Name      Secret     `json:"name"`
		CreatedAt AppletTime `json:"created_at"`
	}

	sa := SA{
		UserID:    123456,
		Name:      "123456sdaf",
		CreatedAt: AppletTime(time.Now()),
	}
	b, err := json.Marshal(sa)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))

	var sa2 SA
	err = json.Unmarshal(b, &sa2)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(sa2)

}

func TestUUID(t *testing.T) {
	t.Logf(hex.EncodeToString(uuid.Must(uuid.NewV4(), nil).Bytes()))
	t.Log(uuid.Must(uuid.NewV4(), nil).String())
}
