package paytype

import (
	"time"

	"github.com/ygpkg/yg-go/types"
	"gorm.io/gorm"
)

// PayRefund 退款表
type PayRefund struct {
	gorm.Model
	// Uin 用户ID
	Uin uint `gorm:"column:uin;type:bigint;not null;comment:用户uin" json:"uin"`
	// CompanyID 公司ID
	CompanyID uint `gorm:"column:company_id;type:bigint;comment:公司id" json:"company_id"`

	// Reason 退款原因
	Reason string `gorm:"column:reason;type:varchar(50);comment:退款原因" json:"reason"`
	// OrderNo 订单号 来自订单表
	OrderNo string `gorm:"column:order_no;type:varchar(32);not null;comment:订单号" json:"order_no"`
	// RefundNo 商户退款号
	RefundNo string `gorm:"column:refund_no;type:varchar(32);not null;comment:商户退款号" json:"refund_no"`
	// Amount 退款金额
	Amount types.Money `gorm:"column:amount;type:float;comment:退款金额" json:"amount"`
	// PayType 支付方式
	PayType PayType `gorm:"column:pay_type;type:varchar(32);not null;comment:支付类型" json:"pay_type"`
	// PayStatus 支付状态
	PayStatus PayStatus `gorm:"column:pay_status;type:varchar(32);not null;comment:支付状态" json:"pay_status"`
	// RefundTime 第三方退款创建时间
	RefundTime *time.Time `gorm:"column:refund_time;type:datetime;comment:第三方支付创建时间" json:"refund_time"`
	// RefundSuccessTime 第三方退款成功时间
	RefundSuccessTime *time.Time `gorm:"column:refund_success_time;type:datetime;comment:第三方退款成功时间" json:"refund_success_time"`
	// RefundReq 发起退款请求体信息
	RefundReq string `gorm:"column:pre_pay_req;type:text;comment:发起退款请求体信息" json:"pre_pay_req"`
	// RefundResp 退款响应体信息
	RefundResp string `gorm:"column:pre_pay_resp;type:text;comment:退款响应体信息" json:"pre_pay_resp"`
}

// TableName 表名
func (PayRefund) TableName() string {
	return TableNamePayRefund
}
