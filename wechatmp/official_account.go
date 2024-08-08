package wechatmp

import (
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/officialaccount"
	offConfig "github.com/silenceper/wechat/v2/officialaccount/config"
	"github.com/ygpkg/yg-go/cache"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

// GetWechatOfficialAccount 获取微信公众号实例
func GetWechatOfficialAccount(group, key string) (*officialaccount.OfficialAccount, error) {
	cfg := &offConfig.Config{}
	if err := settings.GetYaml(group, key, cfg); err != nil {
		logs.Errorf("GetWechatOfficialAccount: get config failed, %s", err)
		return nil, err
	}
	logs.Infof("GetWechatOfficialAccount: %v", cfg.AppID)

	wApp := wechat.NewWechat()
	wApp.SetCache(cache.WechatCache())
	return wApp.GetOfficialAccount(cfg), nil
}
