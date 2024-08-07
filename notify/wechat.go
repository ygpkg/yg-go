package notify

import (
	"github.com/silenceper/wechat/v2"
	"github.com/ygpkg/yg-go/cache"
)

func SendWechatTemplateMsg() {
	wc := wechat.NewWechat()
	wc.SetCache(cache.WechatCache())
}
