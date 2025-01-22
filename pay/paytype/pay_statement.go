package paytype

import (
	"github.com/ygpkg/yg-go/types"
	"gorm.io/gorm"
)

// PayStatement 流水表
type PayStatement struct {
	gorm.Model
	// Uin 用户ID
	Uin uint `gorm:"column:uin;type:bigint;not null;comment:用户uin" json:"uin"`
	// CompanyID 公司ID
	CompanyID uint `gorm:"column:company_id;type:bigint;comment:公司id" json:"company_id"`

	// TransactionType 出账还是入账
	TransactionType TransactionType `gorm:"column:transaction_type;type:varchar(32);not null;comment:出账还是入账" json:"transaction_type"`
	// OrderNo 订单号 来自订单表
	OrderNo string `gorm:"column:order_no;type:varchar(32);not null;comment:订单号" json:"order_no"`
	// SubjectNo 支付号或退款号
	SubjectNo string `gorm:"column:subject_no;type:varchar(32);not null;unique;comment:支付号或退款号" json:"subject_no"`
	// Amount 金额
	Amount types.Money `gorm:"column:amount;type:float;comment:金额" json:"amount"`
}

// TableName 表名
func (PayStatement) TableName() string {
	return TableNamePayStatement
}

// PayWay 出账还是入账
type TransactionType string

// 针对运营商为出账入账，针对用户就相反
const (
	// PayWayIn 入账
	TransactionTypeIn TransactionType = "in"
	// PayWayOut 出账
	TransactionTypeOut TransactionType = "out"
)
