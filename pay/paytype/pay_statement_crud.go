package paytype

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// CreatePayStatement 创建支付流水
func CreatePayStatement(db *gorm.DB, payStatement *PayStatement) error {
	err := db.Create(payStatement).Error
	if err != nil {
		logs.Errorf("CreatePayStatement error: %v", err)
		return err
	}
	return nil
}
