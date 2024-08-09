package notify

import (
	"context"
	"fmt"

	"github.com/silenceper/wechat/v2/officialaccount/message"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/wechatmp"
)

// SendWechatOfficialAccountTemplateMsg 发送微信公众号模板消息
func SendWechatOfficialAccountTemplateMsg(ctx context.Context, group, key, tplname string, msg *message.TemplateMessage) error {
	cfg, mp, err := wechatmp.GetWechatOfficialAccount(group, key)
	if err != nil {
		logs.ErrorContextf(ctx, "SendWechatTemplateMsg: get wechat official account failed, %s", err)
		return err
	}

	if msg.TemplateID == "" {
		msg.TemplateID = cfg.Templates[tplname]
		if msg.TemplateID == "" {
			logs.ErrorContextf(ctx, "SendWechatTemplateMsg: template not found, tplname: %s", tplname)
			return fmt.Errorf("template not found, tplname: %s", tplname)
		}
	}
	msgid, err := mp.GetTemplate().Send(msg)
	if err != nil {
		logs.ErrorContextf(ctx, "SendWechatTemplateMsg: send template message failed, %s", err)
		return err
	}
	logs.Infof("SendWechatTemplateMsg: send template message success, msgid: %v, body: %+v", msgid, msg)
	return nil
}
