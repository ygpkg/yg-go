package auth

import "errors"

type LoginWay uint8 // 登录方式

const (
	LoginWayUnknown LoginWay = 0
	// LoginWayEmail 邮箱+密码
	LoginWayEmail LoginWay = 1
	// LoginWayPhone 手机号+验证码
	LoginWayPhone LoginWay = 2
	// LoginWayWxWeb 微信网页端
	LoginWayWxWeb LoginWay = 3
	// LoginWayWxMp 微信公众号
	LoginWayWxMp LoginWay = 4
	// LoginWayWxMini 微信小程序
	LoginWayWxMini LoginWay = 5
	// LoginWayGithub github
	LoginWayGithub LoginWay = 6
	// LoginWayWorkWechat 企业微信
	LoginWayWorkWechat LoginWay = 7
)

var (
	// ErrInvalidAudience 无效的audience
	ErrInvalidAudience = errors.New("invalid audience")
	// ErrInvalidLoginWay 无效的登录方式
	ErrInvalidLoginWay = errors.New("invalid login way")
)
