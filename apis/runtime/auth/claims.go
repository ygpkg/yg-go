package auth

import (
	"encoding/json"
	"fmt"

	"github.com/dgrijalva/jwt-go"
)

// UserClaims 用户信息
type UserClaims struct {
	// Uin 账号ID，主体+用户对应的唯一ID
	Uin uint `json:"c,omitempty"`
	// Subject 账号主体，资源所有者（冗余）
	// Subject uint `json:"s,omitempty"`

	// IssuedAt 创建时间
	IssuedAt int64 `json:"t,omitempty"`
	// ExpiresAt 过期时间
	ExpiresAt int64 `json:"e,omitempty"`
	// Issuer 签发者 区分不同签发者
	Issuer string `json:"i,omitempty"`
	// Audience 接收者
	Audience string `json:"a,omitempty"`
	// LoginWay 登录方式
	LoginWay LoginWay `json:"l,omitempty"`
	// SessionID 会话 ID：把 token 绑定到一条服务端会话记录，用于踢下线与单点登录。
	SessionID string `json:"sid,omitempty"`
	// SessionVersion 会话版本号：会话被刷新（改密、强制重登）后递增，
	// 校验方拿它比对库里的当前版本，旧 token 立即失效。
	SessionVersion int64 `json:"session_version,omitempty"`
}

// UnmarshalJSON 同时接受两种 claims 编码：
//
//   - yg-go 历史格式（紧凑键）：`c` / `t` / `e` / `i` / `a` / `l`；
//   - 标准 JWT 声明（其他语言的服务签发的 token）：`uin` / `iat` / `exp` / `iss` / `aud` /
//     `login_way`，外加 `sid` / `session_version`。
//
// 为什么必须两种都认：签发方与校验方不一定在同一个语言栈里（例如业务由 Node 签发、
// 由 Go 校验），只认紧凑键会让标准格式的 token 解析成全零 claims → 任何 token 都验不过。
// 标准键只在**非零值**时覆盖紧凑键，这样同一份 payload 里两种键共存也不会互相踩。
func (c *UserClaims) UnmarshalJSON(data []byte) error {
	var raw struct {
		Uin               uint     `json:"c"`
		StandardUin       uint     `json:"uin"`
		IssuedAt          int64    `json:"t"`
		StandardIssuedAt  int64    `json:"iat"`
		ExpiresAt         int64    `json:"e"`
		StandardExpiresAt int64    `json:"exp"`
		Issuer            string   `json:"i"`
		StandardIssuer    string   `json:"iss"`
		Audience          string   `json:"a"`
		StandardAudience  string   `json:"aud"`
		LoginWay          LoginWay `json:"l"`
		StandardLoginWay  LoginWay `json:"login_way"`
		SessionID         string   `json:"sid"`
		SessionVersion    int64    `json:"session_version"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.Uin = raw.Uin
	if raw.StandardUin != 0 {
		c.Uin = raw.StandardUin
	}
	c.IssuedAt = raw.IssuedAt
	if raw.StandardIssuedAt != 0 {
		c.IssuedAt = raw.StandardIssuedAt
	}
	c.ExpiresAt = raw.ExpiresAt
	if raw.StandardExpiresAt != 0 {
		c.ExpiresAt = raw.StandardExpiresAt
	}
	c.Issuer = raw.Issuer
	if raw.StandardIssuer != "" {
		c.Issuer = raw.StandardIssuer
	}
	c.Audience = raw.Audience
	if raw.StandardAudience != "" {
		c.Audience = raw.StandardAudience
	}
	c.LoginWay = raw.LoginWay
	if raw.StandardLoginWay != 0 {
		c.LoginWay = raw.StandardLoginWay
	}
	c.SessionID = raw.SessionID
	c.SessionVersion = raw.SessionVersion
	return nil
}

// Valid time based claims "exp, iat, nbf".
// There is no accounting for clock skew.
// As well, if any of the above claims are not in the token, it will still
// be considered a valid claim.
func (c UserClaims) Valid() error {
	vErr := new(jwt.ValidationError)
	now := jwt.TimeFunc().Unix()

	if c.IssuedAt > now {
		vErr.Inner = fmt.Errorf("token used before issued")
		vErr.Errors |= jwt.ValidationErrorIssuedAt
	}
	if c.ExpiresAt < now {
		vErr.Inner = fmt.Errorf("token is expired")
		vErr.Errors |= jwt.ValidationErrorExpired
	}

	if vErr.Errors == 0 {
		return nil
	}

	return vErr
}
