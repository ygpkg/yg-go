package paytype

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// CreatePayOrder 创建订单
func CreatePayOrder(db *gorm.DB, order *PayOrder) error {
	err := db.Create(order).Error
	if err != nil {
		logs.Errorf("CreatePayOrder error: %v", err)
		return err
	}
	return nil
}

// GetPayOrderByOrderNo 根据订单号获取订单
func GetPayOrderByOrderNo(db *gorm.DB, orderNo string) (*PayOrder, error) {
	var order PayOrder
	err := db.Where("order_no = ?", orderNo).First(&order).Error
	if err != nil {
		logs.Errorf("GetPayOrderByOrderNo error: %v", err)
		return nil, err
	}
	return &order, nil
}

// SavePayOrder 保存订单
func SavePayOrder(db *gorm.DB, order *PayOrder) error {
	err := db.Save(order).Error
	if err != nil {
		logs.Errorf("SavePayOrder error: %v", err)
		return err
	}
	return nil
}
