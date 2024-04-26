package notify

import (
	"github.com/xen0n/go-workwx"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/utils/logs"
)

// SendWecomTextMsg 发送企业微信文本消息
func SendWecomTextMsg(opt config.WecomApp, content string, userlist ...string) error {
	if !opt.IsValide() {
		logs.Errorf("SendWecomTextMsg, invalid wecom app config: %+v", opt)
		return nil
	}
	wapp := workwx.New(opt.CompanyID).
		WithApp(opt.Secret, opt.AgentID)
	err := wapp.SendTextMessage(&workwx.Recipient{
		UserIDs: userlist,
	}, content, false)
	if err != nil {
		logs.Errorf("SendWecomTextMsg, failed to send wecom text message, content: %s, userlist: %v, %s", content, userlist, err)
		return err
	}
	return nil
}
