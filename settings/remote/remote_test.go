package remote

import (
	"context"
	"testing"
)

func TestRemoteGet(t *testing.T) {
	cli := NewRemoteSettingClientWithEnv()
	content, err := cli.Get(context.Background(), "account_main")
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
}
