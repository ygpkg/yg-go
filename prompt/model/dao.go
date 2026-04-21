package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// PromptCond CorePrompt 查询条件，支持按公司、应用、分组、编码、名称模糊、状态等维度筛选
type PromptCond struct {
	CompanyID uint
	Uin       uint
	ID        uint
	IDs       []uint
	App       string
	Group     string
	Code      string
	NameLike  string
	Status    int
	OrderBy   []string
	Offset    int
	Limit     int
	IsDelete  bool
}

// PromptVersionCond CorePromptVersion 查询条件，支持按公司、promptID 等维度筛选
type PromptVersionCond struct {
	CompanyID uint
	Uin       uint
	ID        uint
	IDs       []uint
	PromptID  uint
	PromptIDs []uint
	OrderBy   []string
	Offset    int
	Limit     int
	IsDelete  bool
}

// PromptDao core_prompt 表 DAO，封装 CRUD 与条件查询
type PromptDao struct {
	db *gorm.DB
}

// NewPromptDao 创建 PromptDao 实例
func NewPromptDao(db *gorm.DB) *PromptDao {
	return &PromptDao{db: db}
}

// TableName 返回 PromptDao 操作的表名
func (dao *PromptDao) TableName() string {
	return TableNameCorePrompt
}

// Insert 新增一条 CorePrompt 记录
func (dao *PromptDao) Insert(ctx context.Context, entity *CorePrompt) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Create(entity).Error; err != nil {
		return fmt.Errorf("[PromptDao] Insert fail, err: %v", err)
	}
	return nil
}

// BatchInsert 批量新增 CorePrompt 记录，空列表直接返回 nil
func (dao *PromptDao) BatchInsert(ctx context.Context, entityList CorePromptList) error {
	if len(entityList) == 0 {
		return nil
	}
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Create(entityList).Error; err != nil {
		return fmt.Errorf("[PromptDao] BatchInsert fail, err: %v", err)
	}
	return nil
}

// UpdateByID 按主键更新 CorePrompt 非0字段（struct 映射）
func (dao *PromptDao) UpdateByID(ctx context.Context, id uint, entity *CorePrompt) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Updates(entity).Error; err != nil {
		return fmt.Errorf("[PromptDao] UpdateByID fail, id:%d, err: %v", id, err)
	}
	return nil
}

// UpdateMap 按主键更新 CorePrompt 指定字段（map 映射）
func (dao *PromptDao) UpdateMap(ctx context.Context, id uint, updateMap map[string]interface{}) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Updates(updateMap).Error; err != nil {
		return fmt.Errorf("[PromptDao] UpdateMap fail, id:%d, err: %v", id, err)
	}
	return nil
}

// Delete 按主键软删 CorePrompt（置 deleted_at）
func (dao *PromptDao) Delete(ctx context.Context, id uint) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Updates(map[string]interface{}{"deleted_at": time.Now()}).Error; err != nil {
		return fmt.Errorf("[PromptDao] Delete fail, id:%d, err: %v", id, err)
	}
	return nil
}

// DeleteByIDs 按主键列表批量软删 CorePrompt（置 deleted_at）
func (dao *PromptDao) DeleteByIDs(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id IN ?", ids).Updates(map[string]interface{}{"deleted_at": now}).Error; err != nil {
		return fmt.Errorf("[PromptDao] DeleteByIDs fail, err: %v", err)
	}
	return nil
}

// GetByID 按主键查询单条 CorePrompt
func (dao *PromptDao) GetByID(ctx context.Context, id uint) (*CorePrompt, error) {
	var entity CorePrompt
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetByID fail, id:%d, err: %v", id, err)
	}
	return &entity, nil
}

// GetByCond 按条件查询单条 CorePrompt
func (dao *PromptDao) GetByCond(ctx context.Context, cond *PromptCond) (*CorePrompt, error) {
	var entity CorePrompt
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	dao.BuildCondition(db, cond)
	if err := db.Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return &entity, nil
}

// GetByAppAndGroup 按应用与业务分组查询 CorePrompt
func (dao *PromptDao) GetByAppAndGroup(ctx context.Context, app, group string) (*CorePrompt, error) {
	var entity CorePrompt
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("app = ? AND group = ?", app, group).Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetByAppAndGroup fail, app:%s, group:%s, err: %v", app, group, err)
	}
	return &entity, nil
}

// GetByCode 按业务编码查询 CorePrompt
func (dao *PromptDao) GetByCode(ctx context.Context, code string) (*CorePrompt, error) {
	var entity CorePrompt
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("code = ?", code).Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetByCode fail, code:%s, err: %v", code, err)
	}
	return &entity, nil
}

// GetListByCond 按条件查询 CorePrompt 列表
func (dao *PromptDao) GetListByCond(ctx context.Context, cond *PromptCond) (CorePromptList, error) {
	var entityList CorePromptList
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	dao.BuildCondition(db, cond)
	if err := db.Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetListByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return entityList, nil
}

// GetPageListByCond 按条件分页查询 CorePrompt 列表并返回总数
func (dao *PromptDao) GetPageListByCond(ctx context.Context, cond *PromptCond) (CorePromptList, int64, error) {
	db := dao.db.WithContext(ctx).Model(&CorePrompt{}).Table(dao.TableName())
	dao.BuildCondition(db, cond)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("[PromptDao] GetPageListByCond count fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	if cond.Limit > 0 {
		db.Limit(cond.Limit)
	}
	if cond.Offset > 0 {
		db.Offset(cond.Offset)
	}
	var entityList CorePromptList
	if err := db.Find(&entityList).Error; err != nil {
		return nil, 0, fmt.Errorf("[PromptDao] GetPageListByCond find fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return entityList, count, nil
}

// CountByCond 按条件统计 CorePrompt 总数
func (dao *PromptDao) CountByCond(ctx context.Context, cond *PromptCond) (int64, error) {
	db := dao.db.WithContext(ctx).Model(&CorePrompt{}).Table(dao.TableName())
	dao.BuildCondition(db, cond)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("[PromptDao] CountByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return count, nil
}

// BuildCondition 构建 CorePrompt 查询的 WHERE 条件链，含 company/app/group/code/status/软删等维度
func (dao *PromptDao) BuildCondition(db *gorm.DB, cond *PromptCond) {
	if cond == nil {
		return
	}
	tn := dao.TableName()
	if cond.CompanyID > 0 {
		db.Where(fmt.Sprintf("%s.company_id = ?", tn), cond.CompanyID)
	}
	if cond.Uin > 0 {
		db.Where(fmt.Sprintf("%s.uin = ?", tn), cond.Uin)
	}
	if cond.ID > 0 {
		db.Where(fmt.Sprintf("%s.id = ?", tn), cond.ID)
	}
	if len(cond.IDs) > 0 {
		db.Where(fmt.Sprintf("%s.id IN ?", tn), cond.IDs)
	}
	if cond.App != "" {
		db.Where(fmt.Sprintf("%s.app = ?", tn), cond.App)
	}
	if cond.Group != "" {
		db.Where(fmt.Sprintf("%s.group = ?", tn), cond.Group)
	}
	if cond.Code != "" {
		db.Where(fmt.Sprintf("%s.code = ?", tn), cond.Code)
	}
	if cond.NameLike != "" {
		db.Where(fmt.Sprintf("%s.name LIKE ?", tn), cond.NameLike+"%")
	}
	if cond.Status >= 0 {
		db.Where(fmt.Sprintf("%s.status = ?", tn), cond.Status)
	}
	if cond.IsDelete {
		db.Unscoped()
	}
	if len(cond.OrderBy) > 0 {
		db.Order(strings.Join(cond.OrderBy, ","))
	}
}

// PromptVersionDao core_prompt_version 表 DAO，封装 CRUD 与条件查询
type PromptVersionDao struct {
	db *gorm.DB
}

// NewPromptVersionDao 创建 PromptVersionDao 实例
func NewPromptVersionDao(db *gorm.DB) *PromptVersionDao {
	return &PromptVersionDao{db: db}
}

// TableName 返回 PromptVersionDao 操作的表名
func (dao *PromptVersionDao) TableName() string {
	return TableNameCorePromptVersion
}

// Insert 新增一条 CorePromptVersion 记录
func (dao *PromptVersionDao) Insert(ctx context.Context, entity *CorePromptVersion) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Create(entity).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] Insert fail, err: %v", err)
	}
	return nil
}

// BatchInsert 批量新增 CorePromptVersion 记录，空列表直接返回 nil
func (dao *PromptVersionDao) BatchInsert(ctx context.Context, entityList CorePromptVersionList) error {
	if len(entityList) == 0 {
		return nil
	}
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Create(entityList).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] BatchInsert fail, err: %v", err)
	}
	return nil
}

// UpdateByID 按主键更新 CorePromptVersion 非0字段（struct 映射）
func (dao *PromptVersionDao) UpdateByID(ctx context.Context, id uint, entity *CorePromptVersion) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Updates(entity).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] UpdateByID fail, id:%d, err: %v", id, err)
	}
	return nil
}

// UpdateMap 按主键更新 CorePromptVersion 指定字段（map 映射）
func (dao *PromptVersionDao) UpdateMap(ctx context.Context, id uint, updateMap map[string]interface{}) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Updates(updateMap).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] UpdateMap fail, id:%d, err: %v", id, err)
	}
	return nil
}

// Delete 按主键软删 CorePromptVersion（置 deleted_at）
func (dao *PromptVersionDao) Delete(ctx context.Context, id uint) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Updates(map[string]interface{}{"deleted_at": time.Now()}).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] Delete fail, id:%d, err: %v", id, err)
	}
	return nil
}

// GetByID 按主键查询单条 CorePromptVersion
func (dao *PromptVersionDao) GetByID(ctx context.Context, id uint) (*CorePromptVersion, error) {
	var entity CorePromptVersion
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptVersionDao] GetByID fail, id:%d, err: %v", id, err)
	}
	return &entity, nil
}

// GetByIDs 按主键列表批量查询 CorePromptVersion，空列表直接返回 nil
func (dao *PromptVersionDao) GetByIDs(ctx context.Context, ids []uint) (CorePromptVersionList, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var entityList CorePromptVersionList
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id IN ?", ids).Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[PromptVersionDao] GetByIDs fail, err: %v", err)
	}
	return entityList, nil
}

// GetListByCond 按条件查询 CorePromptVersion 列表
func (dao *PromptVersionDao) GetListByCond(ctx context.Context, cond *PromptVersionCond) (CorePromptVersionList, error) {
	var entityList CorePromptVersionList
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	dao.BuildCondition(db, cond)
	if err := db.Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[PromptVersionDao] GetListByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return entityList, nil
}

// GetPageListByCond 按条件分页查询 CorePromptVersion 列表并返回总数
func (dao *PromptVersionDao) GetPageListByCond(ctx context.Context, cond *PromptVersionCond) (CorePromptVersionList, int64, error) {
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{}).Table(dao.TableName())
	dao.BuildCondition(db, cond)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("[PromptVersionDao] GetPageListByCond count fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	if cond.Limit > 0 {
		db.Limit(cond.Limit)
	}
	if cond.Offset > 0 {
		db.Offset(cond.Offset)
	}
	var entityList CorePromptVersionList
	if err := db.Find(&entityList).Error; err != nil {
		return nil, 0, fmt.Errorf("[PromptVersionDao] GetPageListByCond find fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return entityList, count, nil
}

// GetListByPromptID 按 promptID 查询该模板下所有版本，0 值直接返回 nil
func (dao *PromptVersionDao) GetListByPromptID(ctx context.Context, promptID uint) (CorePromptVersionList, error) {
	if promptID == 0 {
		return nil, nil
	}
	var entityList CorePromptVersionList
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("prompt_id = ?", promptID).Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[PromptVersionDao] GetListByPromptID fail, promptID:%d, err: %v", promptID, err)
	}
	return entityList, nil
}

// BuildCondition 构建 CorePromptVersion 查询的 WHERE 条件链，含 company/promptID/软删等维度
func (dao *PromptVersionDao) BuildCondition(db *gorm.DB, cond *PromptVersionCond) {
	if cond == nil {
		return
	}
	tn := dao.TableName()
	if cond.CompanyID > 0 {
		db.Where(fmt.Sprintf("%s.company_id = ?", tn), cond.CompanyID)
	}
	if cond.Uin > 0 {
		db.Where(fmt.Sprintf("%s.uin = ?", tn), cond.Uin)
	}
	if cond.ID > 0 {
		db.Where(fmt.Sprintf("%s.id = ?", tn), cond.ID)
	}
	if len(cond.IDs) > 0 {
		db.Where(fmt.Sprintf("%s.id IN ?", tn), cond.IDs)
	}
	if cond.PromptID > 0 {
		db.Where(fmt.Sprintf("%s.prompt_id = ?", tn), cond.PromptID)
	}
	if len(cond.PromptIDs) > 0 {
		db.Where(fmt.Sprintf("%s.prompt_id IN ?", tn), cond.PromptIDs)
	}
	if cond.IsDelete {
		db.Unscoped()
	}
	if len(cond.OrderBy) > 0 {
		db.Order(strings.Join(cond.OrderBy, ","))
	}
}
