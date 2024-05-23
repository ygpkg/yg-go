package constants

import "fmt"

const (
	RedisKeyChatMessage      = "subscribe:aigc:chat:question"
	RedisKeyChatMessageQueue = "queue:aigc:chat:question"
)

func RedisKeyWechatWebOauthUserInfo(unionid string) string {
	return fmt.Sprintf("wechat_web:oauth:user:%s", unionid)
}
