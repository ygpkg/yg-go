package paytype

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// CreatePayPayment 创建支付记录
func CreatePayPayment(db *gorm.DB, payment *Payment) error {
	err := db.Create(payment).Error
	if err != nil {
		logs.Errorf("CreatePayPayment error: %v", err)
		return err
	}
	return nil
}

// GetPayPaymentByOrderNo 根据订单号获取支付成功支付记录
func GetPayPaymentByOrderNo(db *gorm.DB, orderNo string) (*Payment, error) {
	var payment Payment
	err := db.Where("order_no = ?", orderNo).Where("pay_status = ?", PayStatusSuccess).First(&payment).Error
	if err != nil {
		logs.Errorf("GetPayPaymentByOrderNo error: %v", err)
		return nil, err
	}
	return &payment, nil
}

// GetPayPayment 根据订单号和状态获取支付记录
func GetPayPayment(db *gorm.DB, orderNo string, pay_status PayStatus) ([]*Payment, error) {
	var payments []*Payment
	err := db.Where("order_no = ?", orderNo).Where("pay_status = ?", pay_status).Find(&payments).Error
	if err != nil {
		logs.Errorf("GetPayPaymentByOrderNo error: %v", err)
		return nil, err
	}
	return payments, nil
}

// SavePayPayment 保存支付记录
func SavePayPayment(db *gorm.DB, payment *Payment) error {
	err := db.Save(payment).Error
	if err != nil {
		logs.Errorf("SavePayPayment error: %v", err)
		return err
	}
	return nil
}
