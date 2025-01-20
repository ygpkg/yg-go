package paytype

import (
	"encoding/json"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/types"
	"gorm.io/gorm"
)

// Payment 支付表
type Payment struct {
	gorm.Model
	// Uin 用户ID
	Uin uint `gorm:"column:uin;type:bigint;not null;comment:用户uin" json:"uin"`
	// CompanyID 公司ID
	CompanyID uint `gorm:"column:company_id;type:bigint;comment:公司id" json:"company_id"`

	// Description 订单描述
	Description string `gorm:"column:description;type:varchar(256);comment:订单描述" json:"description"`
	// OrderNo 订单号 来自订单表
	OrderNo string `gorm:"column:order_no;type:varchar(32);not null;comment:订单号" json:"order_no"`
	// TradeNo 商户支付号
	TradeNo string `gorm:"column:trade_no;type:varchar(32);not null;comment:商户支付号" json:"trade_no"`
	// Amount 支付金额
	Amount types.Money `gorm:"column:amount;type:float;comment:支付金额" json:"amount"`
	// PayStatus 支付状态
	PayStatus PayStatus `gorm:"column:pay_status;type:tinyint;not null;comment:支付状态" json:"pay_status"`
	// PayType 支付类型
	PayType PayType `gorm:"column:pay_type;type:tinyint;not null;comment:支付类型" json:"pay_type"`
	// TradeTpye 调起交易类型
	TradeTpye string `gorm:"column:trade_tpye;type:varchar(32);comment:调起交易类型" json:"trade_tpye"`
	// AppID 应用ID
	AppID string `gorm:"column:app_id;type:varchar(32);comment:应用ID" json:"app_id"`
	// MchID 商户号
	MchID string `gorm:"column:mch_id;type:varchar(32);comment:商户号" json:"mch_id"`
	// TransactionID 第三方交易号
	TransactionID string `gorm:"column:transaction_id;type:varchar(32);comment:第三方交易号" json:"transaction_id"`
	// PayTime 第三方支付创建时间
	PayTime time.Time `gorm:"column:pay_time;type:datetime;comment:第三方支付创建时间" json:"pay_time"`
	// SuccessTime 第三方支付成功时间
	PaySuccessTime time.Time `gorm:"column:pay_success_time;type:datetime;comment:第三方支付成功时间" json:"pay_success_time"`
	// PrePayReq 预支付请求体信息
	PrePayReq string `gorm:"column:pre_pay_req;type:text;comment:预支付请求体信息" json:"pre_pay_req"`
	// PrePayResp 预支付响应体信息
	PrePayResp string `gorm:"column:pre_pay_resp;type:text;comment:预支付响应体信息" json:"pre_pay_resp"`
	// PrePaySign 预支付签名
	PrePaySign string `gorm:"column:pre_pay_sign;type:varchar(256);comment:预支付签名" json:"pre_pay_sign"`
	// ExpireTime 过期时间
	ExpireTime time.Time `gorm:"column:expire_time;type:datetime;comment:过期时间" json:"expire_time"`
}

// TableName 表名
func (Payment) TableName() string {
	return TableNamePayment
}

// JsonString 将req和resp结构体转换为 JSON 字符串
func JsonString(v interface{}) (string, error) {
	// 将结构体转换为 JSON 字符串
	jsonData, err := json.Marshal(v)
	if err != nil {
		logs.Errorf("json marshal failed, %v", err)
		return "", err
	}
	// 转换为字符串
	jsonString := string(jsonData)
	return jsonString, nil
}
