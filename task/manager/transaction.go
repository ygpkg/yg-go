package manager

import (
	"context"

	"gorm.io/gorm"
)

// txKey is the key for the transaction in the context
type txKey struct{}

// WithTx returns a new context with the given transaction
// 将事务对象注入 Context
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// GetTx returns the transaction from the context, or nil if not present
// 从 Context 中获取事务对象
func GetTx(ctx context.Context) *gorm.DB {
	tx, ok := ctx.Value(txKey{}).(*gorm.DB)
	if !ok {
		return nil
	}
	return tx
}
