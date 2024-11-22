package auth

import (
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
