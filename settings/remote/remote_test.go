package remote

import "testing"

func TestRemoteGet(t *testing.T) {
	cli := NewRemoteSettingClientWithEnv()
	content, err := cli.Get("account_main")
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(content)
}
