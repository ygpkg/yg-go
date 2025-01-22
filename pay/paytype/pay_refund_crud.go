package paytype

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// CreatePayRefund 创建订单
func CreatePayRefund(db *gorm.DB, refund *PayRefund) error {
	err := db.Create(refund).Error
	if err != nil {
		logs.Errorf("CreatePayRefund error: %v", err)
		return err
	}
	return nil
}

// GetPayRefundByOrderNo 根据订单号获取退款表信息
func GetPayRefundByOrderNo(db *gorm.DB, orderNo string) (*PayRefund, error) {
	var refund PayRefund
	err := db.Where("order_no = ?", orderNo).First(&refund).Error
	if err != nil {
		logs.Errorf("GetPayOrderByOrderNo error: %v", err)
		return nil, err
	}
	return &refund, nil
}

// SavePayRefund 保存退款表信息
func SavePayRefund(db *gorm.DB, refund *PayRefund) error {
	err := db.Save(refund).Error
	if err != nil {
		logs.Errorf("SavePayRefund error: %v", err)
		return err
	}
	return nil
}

// GetPayRefund 根据订单号和状态获取退款记录
func GetPayRefund(db *gorm.DB, orderNo string, pay_status PayStatus) ([]*PayRefund, error) {
	var refund []*PayRefund
	err := db.Where("order_no = ?", orderNo).Where("pay_status = ?", pay_status).Find(&refund).Error
	if err != nil {
		logs.Errorf("GetPayPaymentByOrderNo error: %v", err)
		return nil, err
	}
	return refund, nil
}

// GetPayRefundByStatus 根据订单号和状态获取退款记录
func GetPayRefundByStatus(db *gorm.DB, orderNo string, pay_status PayStatus) (*PayRefund, error) {
	var refund PayRefund
	err := db.Where("order_no = ?", orderNo).Where("pay_status = ?", pay_status).First(&refund).Error
	if err != nil {
		logs.Errorf("GetPayOrderByOrderNo error: %v", err)
		return nil, err
	}
	return &refund, nil
}
