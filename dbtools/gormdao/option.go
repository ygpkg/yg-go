package gormdao

// Option 用于定制 Dao 的行为。
type Option func(*options)

type options struct {
	isSoftDelete bool
}

// WithoutSoftDelete 关闭软删除：Delete 将通过 Unscoped 物理删除记录。
// 对声明了 gorm.DeletedAt 的实体同样生效，避免退化为 GORM 默认软删除。
func WithoutSoftDelete() Option {
	return func(c *options) {
		c.isSoftDelete = false
	}
}
