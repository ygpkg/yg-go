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

	// TotalAmount 总价
	TotalAmount types.Money `gorm:"column:total_amount;type:float;comment:总价" json:"total_amount"`
	// ShouldAmount 应支付金额
	ShouldAmount types.Money `gorm:"column:should_amount;type:float;comment:总价" json:"should_amount"`
	// OrderNo 订单号
	OrderNo string `gorm:"column:order_no;type:varchar(32);not null;comment:订单号" json:"order_no"`
	// PayType 支付类型
	PayType PayType `gorm:"column:pay_type;type:tinyint;not null;comment:支付类型" json:"pay_type"`
	// PayStatus 支付状态
	PayStatus PayStatus `gorm:"column:pay_status;type:tinyint;not null;comment:支付状态" json:"pay_status"`
	// OrderStatus 订单状态
	OrderStatus OrderStatus `gorm:"column:order_status;type:tinyint;not null;comment:订单状态" json:"order_status"`
	// ExpireTime 过期时间
	ExpireTime time.Time `gorm:"column:expire_time;type:datetime;comment:过期时间" json:"expire_time"`
}

// TableName 表名
func (PayOrder) TableName() string {
	return TableNamePayOrder
}

// PayType 支付类型
type PayType int

const (
	PayTypeNull PayType = iota
	PayTypeWechat
)

// String 返回支付类型名称
func (p PayType) String() string {
	switch p {
	case PayTypeNull:
		return "暂未选择"
	case PayTypeWechat:
		return "微信支付"
	default:
		return "未知支付类型"
	}
}

// PayStatus 支付状态
type PayStatus int

const (
	PayStatusNull          PayStatus = iota
	PayStatusPending                 // 待支付
	PayStatusSuccess                 // 支付成功
	PayStatusCancel                  // 取消支付
	PayStatusFail                    // 失败
	PayStatusPendingRefund           // 待退款
	PayStatusFailRefund              // 退款失败
	PayStatusSuccessRefund           // 退款成功
)

// String 返回支付状态名称
func (p PayStatus) String() string {
	switch p {
	case PayStatusNull:
		return "暂未选择"
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
	OrderStatusNull          OrderStatus = iota
	OrderStatusPendingPay                // 待支付
	OrderStatusPendingSend               // 待发货
	OrderStatusCancel                    // 订单取消
	OrderStatusSuccess                   // 已完成
	OrderStatusPendingRefund             // 待退款
	OrderStatusFailRefund                // 退款失败
	OrderStatusSuccessRefund             // 退款成功
)

// String 返回订单状态名称
func (p OrderStatus) String() string {
	switch p {
	case OrderStatusNull:
		return "暂未选择"
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
