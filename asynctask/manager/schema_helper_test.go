package manager

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// schemaCache 与 schemaNamingStrategy 是 gorm schema.Parse 的必需参数（与 gorm 内部默认一致）。
var schemaCache sync.Map

func schemaNamingStrategy() schema.Namer { return schema.NamingStrategy{} }

// tableNameOf 取实体的表名（用 gorm 的 schema 解析，避免手写 switch 与实体漂移）。
func tableNameOf(t *testing.T, model any) string {
	t.Helper()
	sch, err := schema.Parse(model, &schemaCache, schemaNamingStrategy())
	if err != nil {
		t.Fatalf("parse schema %T: %v", model, err)
	}
	return sch.Table
}
