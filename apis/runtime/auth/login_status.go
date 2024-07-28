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
	StateNil State = iota
	StateSucc
	StateParseFailed
	StateInvalidToken
	StateInvalidUser
)

type Role int

const (
	RoleNil Role = iota
	// RoleUser 普通用户
	RoleUser
	// RoleEmployee 运营用户
	RoleEmployee
	// RoleCustomer 客户
	RoleCustomer
)

// LoginStatus
type LoginStatus struct {
	Token string
	Err   error
	Role  Role
	State State

	idmap map[string]uint
}

func (ls *LoginStatus) SetID(idname string, id uint) {
	if ls.idmap == nil {
		ls.idmap = map[string]uint{}
	}
	ls.idmap[idname] = id
}

func (ls *LoginStatus) GetID(idname string) uint {
	return ls.idmap[idname]
}
