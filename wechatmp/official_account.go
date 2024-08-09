package wechatmp

import (
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/officialaccount"
	"github.com/ygpkg/yg-go/cache"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

// GetWechatOfficialAccount 获取微信公众号实例
func GetWechatOfficialAccount(group, key string) (*config.WechatOfficialAccountConfig, *officialaccount.OfficialAccount, error) {
	cfg := &config.WechatOfficialAccountConfig{}
	if err := settings.GetYaml(group, key, cfg); err != nil {
		logs.Errorf("GetWechatOfficialAccount: get config failed, %s", err)
		return nil, nil, err
	}
	logs.Infof("GetWechatOfficialAccount: %v", cfg.AppID)

	wApp := wechat.NewWechat()
	wApp.SetCache(cache.WechatCache())
	return cfg, wApp.GetOfficialAccount(&cfg.Config), nil
}
