package gormdao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// MaxLimit 分页查询单页最大条数，防止误传超大 limit 导致全表扫描或内存暴涨。
const MaxLimit = 1000

// Entity 数据实体接口，要求提供表名。
type Entity interface {
	TableName() string
}

// Dao 基于 GORM 的通用数据访问对象。
//
// 泛型 T 为实体类型，L 为实体切片类型（如 []T），ID 为主键类型
// （支持 uint / int64 / string 等，见 IDType）。
//
// 软删除约定（默认启用）：
//   - Delete 将 deleted_at 置为当前时间，不物理删除；要求表包含 deleted_at 列
//     （嵌入 gorm.Model 或显式声明均可）。
//   - 查询方法自动追加 "deleted_at IS NULL" 过滤；若实体声明了 gorm.DeletedAt，
//     GORM 自身也会追加该过滤，两者叠加无害。
//   - 通过 Cond.IsDelete（或自定义 Cond 实现 IncludeDeleted 返回 true）可查询已删除记录。
//
// 使用 WithoutSoftDelete() 创建时，Delete 将通过 Unscoped 物理删除，
// 对声明了 gorm.DeletedAt 的实体同样生效，避免退化为 GORM 默认软删除。
type Dao[T Entity, L ~[]T, ID IDType] struct {
	base
	TableName    string
	daoName      string
	isSoftDelete bool
}

// NewDao 创建一个 Dao。tableName 为物理表名，daoName 仅用于日志/错误定位，
// db 为底层连接（可由 dbtools.DB(name) 获取）。
func NewDao[T Entity, L ~[]T, ID IDType](tableName string, daoName string, db *gorm.DB, opts ...Option) *Dao[T, L, ID] {
	cfg := &options{isSoftDelete: true}
	for _, opt := range opts {
		opt(cfg)
	}
	return &Dao[T, L, ID]{
		base:         newBase(db),
		TableName:    tableName,
		daoName:      daoName,
		isSoftDelete: cfg.isSoftDelete,
	}
}

// WithTx 返回绑定指定事务的新 Dao，原 Dao 与事务外的调用不受影响。
func (d *Dao[T, L, ID]) WithTx(tx *gorm.DB) *Dao[T, L, ID] {
	return &Dao[T, L, ID]{
		base:         d.base.withTx(tx),
		TableName:    d.TableName,
		daoName:      d.daoName,
		isSoftDelete: d.isSoftDelete,
	}
}

// deletedScope 追加软删除过滤条件：
//   - 未启用软删除：不追加任何条件；
//   - cond 声明包含已删除记录（IncludeDeleted 返回 true）：Unscoped，取消 GORM 自动过滤；
//   - 默认：追加 "deleted_at IS NULL"。
func (d *Dao[T, L, ID]) deletedScope(db *gorm.DB, cond Cond) *gorm.DB {
	if !d.isSoftDelete {
		return db
	}
	if c, ok := cond.(interface{ IncludeDeleted() bool }); ok && c.IncludeDeleted() {
		return db.Unscoped()
	}
	return db.Where(fmt.Sprintf("%s.deleted_at IS NULL", d.TableName))
}

// Insert 插入单条记录。
func (d *Dao[T, L, ID]) Insert(ctx context.Context, entity *T) error {
	db := d.DB(ctx).Table(d.TableName)
	if err := db.Create(entity).Error; err != nil {
		return fmt.Errorf("[%s] Insert fail, entity:%s: %w", d.daoName, logs.JSON(entity), err)
	}
	return nil
}

// BatchInsert 批量插入；空列表视为 no-op，直接返回 nil。
func (d *Dao[T, L, ID]) BatchInsert(ctx context.Context, entityList L) error {
	if len(entityList) == 0 {
		return nil
	}

	db := d.DB(ctx).Table(d.TableName)
	if err := db.Create(entityList).Error; err != nil {
		return fmt.Errorf("[%s] BatchInsert fail, entityList:%s: %w", d.daoName, logs.JSON(entityList), err)
	}
	return nil
}

// UpdateByID 按主键更新。注意：GORM 的 Updates 对 struct 会跳过零值字段，
// 需要写入零值字段时请使用 UpdateMap。
func (d *Dao[T, L, ID]) UpdateByID(ctx context.Context, id ID, entity *T) error {
	db := d.DB(ctx).Model(new(T)).Table(d.TableName)
	if err := db.Where("id = ?", id).Updates(entity).Error; err != nil {
		return fmt.Errorf("[%s] UpdateByID fail, id:%v, entity:%s: %w", d.daoName, id, logs.JSON(entity), err)
	}
	return nil
}

// UpdateMap 按主键用 map 更新，map 中的键值全部写入（含零值）。
func (d *Dao[T, L, ID]) UpdateMap(ctx context.Context, id ID, updateMap map[string]any) error {
	db := d.DB(ctx).Model(new(T)).Table(d.TableName)
	if err := db.Where("id = ?", id).Updates(updateMap).Error; err != nil {
		return fmt.Errorf("[%s] UpdateMap fail, id:%v, updateMap:%s: %w", d.daoName, id, logs.JSON(updateMap), err)
	}
	return nil
}

// Delete 删除记录：
//   - 启用软删除（默认）：将 deleted_at 置为当前时间，不物理删除；
//   - 未启用软删除（WithoutSoftDelete）：Unscoped 物理删除。
func (d *Dao[T, L, ID]) Delete(ctx context.Context, id ID) error {
	db := d.DB(ctx).Model(new(T)).Table(d.TableName)
	if !d.isSoftDelete {
		if err := db.Unscoped().Where("id = ?", id).Delete(new(T)).Error; err != nil {
			return fmt.Errorf("[%s] HardDelete fail, id:%v: %w", d.daoName, id, err)
		}
		return nil
	}

	updatedField := map[string]any{
		"deleted_at": time.Now(),
	}
	if err := db.Where("id = ?", id).Updates(updatedField).Error; err != nil {
		return fmt.Errorf("[%s] Delete fail, id:%v: %w", d.daoName, id, err)
	}
	return nil
}

// GetByID 按主键查询，仅返回未删除记录；不存在时返回 (nil, nil)。
func (d *Dao[T, L, ID]) GetByID(ctx context.Context, id ID) (*T, error) {
	var entity T
	db := d.DB(ctx).Table(d.TableName)
	if d.isSoftDelete {
		db = db.Where(fmt.Sprintf("%s.deleted_at IS NULL", d.TableName))
	}
	if err := db.Where("id = ?", id).Take(&entity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil //nolint:nilnil // 未找到记录约定返回 (nil, nil)
		}
		return nil, fmt.Errorf("[%s] GetByID fail, id:%v: %w", d.daoName, id, err)
	}
	return &entity, nil
}

// GetByCond 按条件查询单条（LIMIT 1），仅返回未删除记录；不存在时返回 (nil, nil)。
func (d *Dao[T, L, ID]) GetByCond(ctx context.Context, cond Cond) (*T, error) {
	var entity T
	db := d.deletedScope(d.DB(ctx).Table(d.TableName), cond)
	cond.BuildCondition(db, d.TableName)
	if err := db.Take(&entity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil //nolint:nilnil // 未找到记录约定返回 (nil, nil)
		}
		return nil, fmt.Errorf("[%s] GetByCond fail: %w", d.daoName, err)
	}
	return &entity, nil
}

// GetListByCond 按条件查询列表，仅返回未删除记录。
func (d *Dao[T, L, ID]) GetListByCond(ctx context.Context, cond Cond) (L, error) {
	var entityList L
	db := d.deletedScope(d.DB(ctx).Table(d.TableName), cond)
	cond.BuildCondition(db, d.TableName)
	if err := db.Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[%s] GetListByCond fail: %w", d.daoName, err)
	}
	return entityList, nil
}

// GetPageListByCond 按条件分页查询，返回当前页数据与总条数（已排除已删除记录）。
// 当 offset/limit 未设置（<=0）时不分页，返回全部记录（含 count）；
// limit 超过 MaxLimit 时按 MaxLimit 截断。
func (d *Dao[T, L, ID]) GetPageListByCond(ctx context.Context, cond Cond) (L, int64, error) {
	offset, limit := cond.GetOffsetInfo()
	if limit > MaxLimit {
		limit = MaxLimit
	}

	db := d.deletedScope(d.DB(ctx).Model(new(T)).Table(d.TableName), cond)
	cond.BuildCondition(db, d.TableName)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("[%s] GetPageListByCond count fail: %w", d.daoName, err)
	}

	if limit > 0 {
		db = db.Limit(limit)
	}
	if offset > 0 {
		db = db.Offset(offset)
	}

	var entityList L
	if err := db.Find(&entityList).Error; err != nil {
		return nil, 0, fmt.Errorf("[%s] GetPageListByCond find fail: %w", d.daoName, err)
	}
	return entityList, count, nil
}

// CountByCond 按条件统计记录数（已排除已删除记录）。
func (d *Dao[T, L, ID]) CountByCond(ctx context.Context, cond Cond) (int64, error) {
	db := d.deletedScope(d.DB(ctx).Model(new(T)).Table(d.TableName), cond)
	cond.BuildCondition(db, d.TableName)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("[%s] CountByCond fail: %w", d.daoName, err)
	}
	return count, nil
}
