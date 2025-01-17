package pay

import (
	"context"
	"testing"
)

func TestNewOrderNo(t *testing.T) {
	for i := 0; i < 100; i++ {
		// 生成100个订单号
		no, err := NewOrderNo(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		t.Log(no)
	}
}

func TestNewTradeNo(t *testing.T) {
	no, err := NewOrderNo(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	NewTradeNo(context.Background(), no)
}
