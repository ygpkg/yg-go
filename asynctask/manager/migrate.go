package manager

import (
	"fmt"

	"github.com/ygpkg/yg-go/asynctask"
	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"
	"gorm.io/gorm"
)

// SchemaModels 返回本组件拥有的全部实体（AutoMigrate 与文档 DDL 生成器共用这一份清单）。
func SchemaModels() []any {
	return []any{&sourcedb.CoreAsyncTask{}, &sourcedb.CoreAsyncTaskRun{}, &sourcedb.CoreAsyncTaskType{}}
}

// AutoMigrate 建表入口（幂等）。New 默认就会调用它。
//
// 替代源 golib 的 internal/tablekit：nil 校验 + db.AutoMigrate + 带 asynctask: 前缀的错误。
func AutoMigrate(gormDB *gorm.DB) error {
	if gormDB == nil {
		return asynctask.ErrNilDB
	}
	if err := gormDB.AutoMigrate(SchemaModels()...); err != nil {
		return fmt.Errorf("asynctask: auto migrate: %w", err)
	}
	return nil
}
