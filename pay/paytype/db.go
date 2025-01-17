package paytype

import (
	"github.com/ygpkg/yg-go/dbtools"
	"gorm.io/gorm"
)

const (
	// 订单表
	TableNamePayOrder = "core_pay_order"
)

func InitDB(db *gorm.DB) error {
	return dbtools.InitModel(db,
		&PayOrder{},
	)
}
