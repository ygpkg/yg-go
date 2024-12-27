package auth

import (
	"github.com/gin-gonic/gin"
)

const (
	AuthBearer = "Bearer "
)

type InjectorFunc func(ctx *gin.Context, ls *LoginStatus) (err error)

type State int

const (
	StateNil    State = 0
	StateSucc   State = 1
	StateFailed State = 2
	// StateParseFailed
	// StateInvalidToken
	// StateInvalidUser
)

type Role int

const (
	RoleNil Role = iota
	// RoleUser 普通用户
	RoleUser
	// RoleEmployee 运营用户
	RoleEmployee
)

// LoginStatus 登录状态
type LoginStatus struct {
	Token string
	Claim *UserClaims
	Err   error
	Role  Role
	State State

	idmap map[string]uint
}

// SetID 设置ID
func (ls *LoginStatus) SetID(idname string, id uint) {
	if ls.idmap == nil {
		ls.idmap = map[string]uint{}
	}
	ls.idmap[idname] = id
}

// GetID 获取ID
func (ls *LoginStatus) GetID(idname string) uint {
	return ls.idmap[idname]
}
