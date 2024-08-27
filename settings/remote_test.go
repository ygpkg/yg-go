package settings

import "testing"

// "https://yygu.cn/v2/cook.GetSettingContent", "AK1yrh4XGgkTmYSom66rdx7mja5295Lo", "SKRJZOTJW5wm7wJkl0OnoB0a67RDEOrU", "roc.prod"
func TestRemoteGet(t *testing.T) {
	cli := NewRemoteSettingClientWithEnv()
	content, err := cli.Get("account_main")
	if err != nil {
		t.Error(err)
	}
	t.Log(content)
}
