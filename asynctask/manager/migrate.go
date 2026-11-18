package manager

import (
	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"
)

// SchemaModels 返回本组件拥有的全部实体清单。
//
// 它是「实体即真源」的唯一清单：DDL 对账测试（asynctask/ddl）与调用方自检都用它。
// 本函数不执行任何 DDL——建表的唯一入口是 asynctask/ddl 的 SQL 文件。
func SchemaModels() []any {
	return []any{&sourcedb.CoreAsyncTask{}, &sourcedb.CoreAsyncTaskRun{}, &sourcedb.CoreAsyncTaskType{}}
}
