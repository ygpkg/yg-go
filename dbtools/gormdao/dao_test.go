package gormdao

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// softEntity 模拟表内存在 deleted_at 列、但模型未声明 gorm.DeletedAt 的实体，
// 用于验证 Dao 层自带的软删除过滤逻辑（此时 GORM 不会自动过滤）。
type softEntity struct {
	ID        uint       `gorm:"primarykey"`
	Name      string     `gorm:"column:name"`
	DeletedAt *time.Time `gorm:"column:deleted_at"` // 普通列，非 gorm.DeletedAt
}

func (softEntity) TableName() string { return "test_soft_entities" }

// gormModelEntity 声明了 gorm.DeletedAt（标准软删除），GORM 会对其查询自动追加 deleted_at IS NULL。
type gormModelEntity struct {
	gorm.Model
	Name string `gorm:"column:name"`
}

func (gormModelEntity) TableName() string { return "test_gorm_model_entities" }

// stringIDEntity 使用 string 主键的实体（内嵌 BaseEntity 自动生成 UUID 主键），
// 验证 Dao 泛型 ID 支持字符串主键。
type stringIDEntity struct {
	BaseEntity
	Name string `gorm:"column:name"`
}

func (stringIDEntity) TableName() string { return "test_string_id_entities" }

// customCond 内嵌 BaseCond 的自定义条件，验证软删除过滤对自定义 Cond 生效。
type customCond struct {
	BaseCond
	Name string
}

func (c *customCond) BuildCondition(db *gorm.DB, tableName string) {
	c.BaseCond.BuildCondition(db, tableName)
	if c.Name != "" {
		db.Where(fmt.Sprintf("%s.name = ?", tableName), c.Name)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	if !assert.NoError(t, db.AutoMigrate(&softEntity{}, &gormModelEntity{}, &stringIDEntity{})) {
		t.FailNow()
	}
	return db
}

func TestDao_SoftDelete_FiltersDeletedRows(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[softEntity, []softEntity, uint]("test_soft_entities", "test", db)
	ctx := context.Background()

	e := &softEntity{Name: "a"}
	assert.NoError(t, dao.Insert(ctx, e))
	id := e.ID
	assert.NotZero(t, id)

	got, err := dao.GetByID(ctx, id)
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Equal(t, "a", got.Name)
	}

	// 软删除：deleted_at 写入，不物理删除
	assert.NoError(t, dao.Delete(ctx, id))

	var raw softEntity
	assert.NoError(t, db.Table("test_soft_entities").Where("id = ?", id).First(&raw).Error)
	assert.NotNil(t, raw.DeletedAt)

	// 各查询方法均排除已删除记录
	got, err = dao.GetByID(ctx, id)
	assert.NoError(t, err)
	assert.Nil(t, got)

	list, err := dao.GetListByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	assert.Empty(t, list)

	count, err := dao.CountByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	assert.Zero(t, count)

	pageList, total, err := dao.GetPageListByCond(ctx, &BaseCond{Offset: 0, Limit: 10})
	assert.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, pageList)
}

func TestDao_SoftDelete_IncludeDeleted(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[softEntity, []softEntity, uint]("test_soft_entities", "test", db)
	ctx := context.Background()

	assert.NoError(t, dao.Insert(ctx, &softEntity{Name: "keep"}))
	deleted := &softEntity{Name: "gone"}
	assert.NoError(t, dao.Insert(ctx, deleted))
	assert.NoError(t, dao.Delete(ctx, deleted.ID))

	// 默认排除已删除
	list, err := dao.GetListByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	if assert.Len(t, list, 1) {
		assert.Equal(t, "keep", list[0].Name)
	}

	// IsDelete=true 时包含已删除
	list, err = dao.GetListByCond(ctx, &BaseCond{IsDelete: true})
	assert.NoError(t, err)
	assert.Len(t, list, 2)

	// 自定义 Cond 内嵌 BaseCond，自动继承 IncludeDeleted
	list, err = dao.GetListByCond(ctx, &customCond{BaseCond: BaseCond{IsDelete: true}, Name: "gone"})
	assert.NoError(t, err)
	if assert.Len(t, list, 1) {
		assert.Equal(t, "gone", list[0].Name)
	}
}

func TestDao_SoftDelete_GormModelEntity(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[gormModelEntity, []gormModelEntity, uint]("test_gorm_model_entities", "test", db)
	ctx := context.Background()

	e := &gormModelEntity{Name: "gm"}
	assert.NoError(t, dao.Insert(ctx, e))
	assert.NoError(t, dao.Delete(ctx, e.ID))

	// GORM 自动过滤 + Dao 层手动过滤叠加，结果一致
	got, err := dao.GetByID(ctx, e.ID)
	assert.NoError(t, err)
	assert.Nil(t, got)

	list, err := dao.GetListByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	assert.Empty(t, list)

	// deleted_at 已写入（软删除不物理删除，需 Unscoped 查询原始行）
	var raw gormModelEntity
	assert.NoError(t, db.Unscoped().Table("test_gorm_model_entities").Where("id = ?", e.ID).First(&raw).Error)
	assert.NotNil(t, raw.DeletedAt)

	// 包含已删除时可查询
	list, err = dao.GetListByCond(ctx, &BaseCond{IsDelete: true})
	assert.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestDao_HardDelete_WithGormModel(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[gormModelEntity, []gormModelEntity, uint]("test_gorm_model_entities", "test",
		db, WithoutSoftDelete())
	ctx := context.Background()

	e := &gormModelEntity{Name: "hard"}
	assert.NoError(t, dao.Insert(ctx, e))
	assert.NoError(t, dao.Delete(ctx, e.ID))

	// 物理删除：即便实体声明了 gorm.DeletedAt，Unscoped 后行已不存在
	var raw gormModelEntity
	err := db.Unscoped().Table("test_gorm_model_entities").Where("id = ?", e.ID).First(&raw).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	got, err := dao.GetByID(ctx, e.ID)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestDao_BatchInsert_EmptyListIsNoop(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[softEntity, []softEntity, uint]("test_soft_entities", "test", db)
	ctx := context.Background()

	assert.NoError(t, dao.BatchInsert(ctx, nil))
	assert.NoError(t, dao.BatchInsert(ctx, []softEntity{}))

	list, err := dao.GetListByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	assert.Empty(t, list)
}

func TestDao_GetByID_NotFound(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[softEntity, []softEntity, uint]("test_soft_entities", "test", db)

	got, err := dao.GetByID(context.Background(), 999)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestDao_GetByCond_NotFound(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[softEntity, []softEntity, uint]("test_soft_entities", "test", db)

	got, err := dao.GetByCond(context.Background(), &BaseCond{IDs: []any{999}})
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestDao_GetPageListByCond_Pagination(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[softEntity, []softEntity, uint]("test_soft_entities", "test", db)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		assert.NoError(t, dao.Insert(ctx, &softEntity{Name: "n"}))
	}
	// 删除一条，验证 count 与列表都排除
	list, err := dao.GetListByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	if assert.Len(t, list, 5) {
		assert.NoError(t, dao.Delete(ctx, list[0].ID))
	}

	// offset/limit 分页：跳过 2 条取 2 条
	pageList, total, err := dao.GetPageListByCond(ctx, &BaseCond{Offset: 2, Limit: 2})
	assert.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, pageList, 2)

	// limit 超上限时截断
	pageList, total, err = dao.GetPageListByCond(ctx, &BaseCond{Limit: MaxLimit + 100})
	assert.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, pageList, 4)

	// offset/limit 非正数时返回全部（历史兼容行为）
	pageList, total, err = dao.GetPageListByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, pageList, 4)
}

func TestDao_WithTx_Rollback(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[softEntity, []softEntity, uint]("test_soft_entities", "test", db)
	ctx := context.Background()

	tx := db.Begin()
	txDao := dao.WithTx(tx)
	assert.NoError(t, txDao.Insert(ctx, &softEntity{Name: "in-tx"}))
	assert.NoError(t, tx.Rollback().Error)

	got, err := dao.GetByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	assert.Nil(t, got)

	// 事务内提交可查询
	tx = db.Begin()
	txDao = dao.WithTx(tx)
	assert.NoError(t, txDao.Insert(ctx, &softEntity{Name: "committed"}))
	assert.NoError(t, tx.Commit().Error)

	got, err = dao.GetByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Equal(t, "committed", got.Name)
	}
}

// TestDao_StringIDEntity 验证 string 主键全链路：BeforeCreate 自动生成 ID、
// GetByID/UpdateByID/UpdateMap/Delete 及 BaseCond 按 ID/IDs 过滤。
func TestDao_StringIDEntity(t *testing.T) {
	db := newTestDB(t)
	dao := NewDao[stringIDEntity, []stringIDEntity, string]("test_string_id_entities", "test", db)
	ctx := context.Background()

	// 未显式设置 ID 时由 BeforeCreate 自动生成（UUID v7）
	e := &stringIDEntity{Name: "auto"}
	assert.NoError(t, dao.Insert(ctx, e))
	assert.NotEmpty(t, e.ID)

	// 显式指定 ID 不被覆盖
	e2 := &stringIDEntity{BaseEntity: BaseEntity{StringID: StringID{ID: "fixed-id"}}, Name: "fixed"}
	assert.NoError(t, dao.Insert(ctx, e2))

	got, err := dao.GetByID(ctx, e2.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Equal(t, "fixed", got.Name)
	}

	// 不存在时返回 nil
	got, err = dao.GetByID(ctx, "not-exist")
	assert.NoError(t, err)
	assert.Nil(t, got)

	// UpdateByID / UpdateMap
	assert.NoError(t, dao.UpdateByID(ctx, e2.ID, &stringIDEntity{Name: "renamed"}))
	got, err = dao.GetByID(ctx, e2.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Equal(t, "renamed", got.Name)
	}

	assert.NoError(t, dao.UpdateMap(ctx, e2.ID, map[string]any{"name": "map-name"}))
	got, err = dao.GetByID(ctx, e2.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, got) {
		assert.Equal(t, "map-name", got.Name)
	}

	// BaseCond 按 ID / IDs 过滤
	list, err := dao.GetListByCond(ctx, &BaseCond{ID: "fixed-id"})
	assert.NoError(t, err)
	if assert.Len(t, list, 1) {
		assert.Equal(t, "map-name", list[0].Name)
	}

	list, err = dao.GetListByCond(ctx, &BaseCond{IDs: []any{"fixed-id"}})
	assert.NoError(t, err)
	assert.Len(t, list, 1)

	// 空字符串 ID 视为未设置，不生成条件
	list, err = dao.GetListByCond(ctx, &BaseCond{})
	assert.NoError(t, err)
	assert.Len(t, list, 2)

	// Delete（软删除）
	assert.NoError(t, dao.Delete(ctx, e2.ID))
	got, err = dao.GetByID(ctx, e2.ID)
	assert.NoError(t, err)
	assert.Nil(t, got)

	list, err = dao.GetListByCond(ctx, &BaseCond{IsDelete: true})
	assert.NoError(t, err)
	assert.Len(t, list, 2)
}
