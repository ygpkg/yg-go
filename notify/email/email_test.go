package email

import (
	"os"
	"testing"
)

func TestSendEmail(t *testing.T) {
	var opt = SMTPOption{
		Addr:     os.Getenv("TEST_SMTP_ADDR"),
		Username: os.Getenv("TEST_SMTP_USERNAME"),
		Password: os.Getenv("TEST_SMTP_PASSWORD"),
		Nickname: os.Getenv("TEST_SMTP_NICKNAME"),
	}
	if err := opt.Validity(); err != nil {
		t.Logf("invalid smtp option, skip test, %s", err)
		return
	}

	acc, err := NewAccount(opt)
	if err != nil {
		t.Fatal(err)
	}
	err = acc.SendHTML("测试标题", `<h1>测试内容</h1><h2>测试内容</h2><h3>测试内容</h3>`, "me@ckeyer.com")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("发送成功")
}
