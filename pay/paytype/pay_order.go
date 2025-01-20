package paytype

import (
	"time"

	"github.com/ygpkg/yg-go/types"
	"gorm.io/gorm"
)

// PayOrder 订单表
type PayOrder struct {
	gorm.Model
	// Uin 用户ID
	Uin uint `gorm:"column:uin;type:bigint;not null;comment:用户uin" json:"uin"`
	// CompanyID 公司ID
	CompanyID uint `gorm:"column:company_id;type:bigint;comment:公司id" json:"company_id"`

	// OrderNo 订单号
	OrderNo string `gorm:"column:order_no;type:varchar(32);not null;comment:订单号" json:"order_no"`
	// Description 订单描述
	Description string `gorm:"column:description;type:varchar(256);comment:订单描述" json:"description"`
	// TotalAmount 总价
	TotalAmount types.Money `gorm:"column:total_amount;type:float;comment:总价" json:"total_amount"`
	// ShouldAmount 应支付金额
	ShouldAmount types.Money `gorm:"column:should_amount;type:float;comment:总价" json:"should_amount"`
	// PayType 支付类型
	PayType PayType `gorm:"column:pay_type;type:tinyint;not null;comment:支付类型" json:"pay_type"`
	// PayStatus 支付状态
	PayStatus PayStatus `gorm:"column:pay_status;type:tinyint;not null;comment:支付状态" json:"pay_status"`
	// OrderStatus 订单状态
	OrderStatus OrderStatus `gorm:"column:order_status;type:tinyint;not null;comment:订单状态" json:"order_status"`
	// ExpireTime 过期时间
	ExpireTime time.Time `gorm:"column:expire_time;type:datetime;comment:过期时间" json:"expire_time"`
	// OrderSnapshot 订单快照
	OrderSnapshot string `gorm:"column:order_snapshot;type:text;comment:订单快照" json:"order_snapshot"`
}

// TableName 表名
func (PayOrder) TableName() string {
	return TableNamePayOrder
}

// PayType 支付类型
type PayType int

const (
	// 微信支付
	PayTypeWechat PayType = 1
)

// String 返回支付类型名称
func (p PayType) String() string {
	switch p {
	case PayTypeWechat:
		return "微信支付"
	default:
		return "未知支付类型"
	}
}

// PayStatus 支付状态
type PayStatus int

const (
	// 待支付
	PayStatusPending PayStatus = 1
	// 支付成功
	PayStatusSuccess PayStatus = 2
	// 取消支付
	PayStatusCancel PayStatus = 3
	// 失败
	PayStatusFail PayStatus = 4
	// 待退款
	PayStatusPendingRefund PayStatus = 5
	// 退款失败
	PayStatusFailRefund PayStatus = 6
	// 退款成功
	PayStatusSuccessRefund PayStatus = 7
)

// String 返回支付状态名称
func (p PayStatus) String() string {
	switch p {
	case PayStatusPending:
		return "待支付"
	case PayStatusSuccess:
		return "支付成功"
	case PayStatusCancel:
		return "取消支付"
	case PayStatusFail:
		return "失败"
	case PayStatusPendingRefund:
		return "待退款"
	case PayStatusFailRefund:
		return "退款失败"
	case PayStatusSuccessRefund:
		return "退款成功"
	default:
		return "未知支付状态"
	}
}

// OrderStatus 订单状态
type OrderStatus int

const (
	// 待支付
	OrderStatusPendingPay OrderStatus = 1
	// 待发货
	OrderStatusPendingSend OrderStatus = 2
	// 订单取消
	OrderStatusCancel OrderStatus = 3
	// 已完成
	OrderStatusSuccess OrderStatus = 4
	// 待退款
	OrderStatusPendingRefund OrderStatus = 5
	// 退款失败
	OrderStatusFailRefund OrderStatus = 6
	// 退款成功
	OrderStatusSuccessRefund OrderStatus = 7
)

// String 返回订单状态名称
func (p OrderStatus) String() string {
	switch p {
	case OrderStatusPendingPay:
		return "待支付"
	case OrderStatusPendingSend:
		return "待发货"
	case OrderStatusCancel:
		return "订单取消"
	case OrderStatusSuccess:
		return "已完成"
	case OrderStatusPendingRefund:
		return "待退款"
	case OrderStatusFailRefund:
		return "退款失败"
	case OrderStatusSuccessRefund:
		return "退款成功"
	default:
		return "未知订单状态"
	}
}
