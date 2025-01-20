package paytype

import (
	"github.com/ygpkg/yg-go/dbtools"
	"gorm.io/gorm"
)

const (
	// TableNamePayOrder 订单表
	TableNamePayOrder = "core_pay_order"
	// TableNamePayOrder 支付表
	TableNamePayment = "core_pay_payment"
	// TableNamePayRefund 退款表
	TableNamePayRefund = "core_pay_refund"
	// TableNamePayStatement 流水表
	TableNamePayStatement = "core_pay_statement"
)

func InitDB(db *gorm.DB) error {
	return dbtools.InitModel(db,
		&PayOrder{},
		&Payment{},
		&PayRefund{},
		&PayStatement{},
	)
}
