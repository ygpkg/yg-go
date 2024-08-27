package remote

import "testing"

func TestRemoteGet(t *testing.T) {
	cli := NewRemoteSettingClientWithEnv()
	content, err := cli.Get("account_main")
	if err != nil {
		t.Error(err)
	}
	t.Log(content)
}
