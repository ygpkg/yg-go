package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/ygpkg/yg-go/dbtools"
)

// ---------- 测试开始 ----------

func TestFileQuery_First(t *testing.T) {

	dbConfigStr := ""
	error := dbtools.InitMutilMySQL(map[string]string{
		"core": dbConfigStr,
	})
	if error != nil {
		t.Skip("skip test, init db error")
		return
	}
	InitDB(dbtools.Core())
	db := dbtools.Core()

	file, err := NewFileQuery(db).
		Hash("md5:97ed5a122a7091e9289bd11d33c24be9").
		Status(FileStatusNormal).
		First()

	assert.NoError(t, err)
	if file != nil {

		assert.Equal(t, uint(836), file.Uin)
	}
}
