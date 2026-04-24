package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	Status    PromptStatus
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

// NewPromptDao creates a PromptDao instance with the given db connection
func NewPromptDao(db *gorm.DB) *PromptDao {
	return &PromptDao{db: db}
}

// TableName returns the table name that PromptDao operates on
func (dao *PromptDao) TableName() string {
	return TableNameCorePrompt
}

// DB returns the underlying gorm.DB instance
func (dao *PromptDao) DB() *gorm.DB {
	return dao.db
}

// WithTx returns a new PromptDao that uses the given transaction db
func (dao *PromptDao) WithTx(tx *gorm.DB) *PromptDao {
	return &PromptDao{db: tx}
}

// Create inserts a new CorePrompt record
func (dao *PromptDao) Create(ctx context.Context, entity *CorePrompt) error {
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Create(entity).Error; err != nil {
		return fmt.Errorf("[PromptDao] Create fail, err: %v", err)
	}
	return nil
}

// CreateBatch batch inserts CorePrompt records, empty list returns nil directly
func (dao *PromptDao) CreateBatch(ctx context.Context, entityList CorePromptList) error {
	if len(entityList) == 0 {
		return nil
	}
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Create(entityList).Error; err != nil {
		return fmt.Errorf("[PromptDao] CreateBatch fail, err: %v", err)
	}
	return nil
}

// GetByID queries a single CorePrompt by primary key
func (dao *PromptDao) GetByID(ctx context.Context, id uint) (*CorePrompt, error) {
	var entity CorePrompt
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Where("id = ?", id).Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetByID fail, id:%d, err: %v", id, err)
	}
	return &entity, nil
}

// GetByIDs queries CorePrompt records by primary key list, empty list returns nil
func (dao *PromptDao) GetByIDs(ctx context.Context, ids []uint) (CorePromptList, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var entityList CorePromptList
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Where("id IN ?", ids).Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetByIDs fail, err: %v", err)
	}
	return entityList, nil
}

// UpdateByID updates CorePrompt non-zero fields by primary key (struct mapping)
func (dao *PromptDao) UpdateByID(ctx context.Context, id uint, entity *CorePrompt) error {
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Where("id = ?", id).Updates(entity).Error; err != nil {
		return fmt.Errorf("[PromptDao] UpdateByID fail, id:%d, err: %v", id, err)
	}
	return nil
}

// UpdateMap updates CorePrompt specified fields by primary key (map mapping)
func (dao *PromptDao) UpdateMap(ctx context.Context, id uint, updateMap map[string]interface{}) error {
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Where("id = ?", id).Updates(updateMap).Error; err != nil {
		return fmt.Errorf("[PromptDao] UpdateMap fail, id:%d, err: %v", id, err)
	}
	return nil
}

// UpdateMapBatch batch updates CorePrompt specified fields by condition
func (dao *PromptDao) UpdateMapBatch(ctx context.Context, cond *PromptCond, updateMap map[string]interface{}) error {
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	dao.BuildCondition(db, cond)
	if err := db.Updates(updateMap).Error; err != nil {
		return fmt.Errorf("[PromptDao] UpdateMapBatch fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return nil
}

// SaveBatchOnConflict batch saves CorePrompt records, upserting on conflict
func (dao *PromptDao) SaveBatchOnConflict(ctx context.Context, entityList CorePromptList, conflictColumns ...string) error {
	if len(entityList) == 0 {
		return nil
	}
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if len(conflictColumns) > 0 {
		cols := make([]clause.Column, 0, len(conflictColumns))
		for _, c := range conflictColumns {
			cols = append(cols, clause.Column{Name: c})
		}
		db = db.Clauses(clause.OnConflict{Columns: cols, UpdateAll: true})
	}
	if err := db.Create(entityList).Error; err != nil {
		return fmt.Errorf("[PromptDao] SaveBatchOnConflict fail, err: %v", err)
	}
	return nil
}

// Delete soft-deletes CorePrompt by primary key (sets deleted_at)
func (dao *PromptDao) Delete(ctx context.Context, id uint) error {
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Where("id = ?", id).Updates(map[string]interface{}{"deleted_at": time.Now()}).Error; err != nil {
		return fmt.Errorf("[PromptDao] Delete fail, id:%d, err: %v", id, err)
	}
	return nil
}

// DeleteByIDs soft-deletes CorePrompt by primary key list (sets deleted_at)
func (dao *PromptDao) DeleteByIDs(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Where("id IN ?", ids).Updates(map[string]interface{}{"deleted_at": now}).Error; err != nil {
		return fmt.Errorf("[PromptDao] DeleteByIDs fail, err: %v", err)
	}
	return nil
}

// GetByCond queries a single CorePrompt by condition
func (dao *PromptDao) GetByCond(ctx context.Context, cond *PromptCond) (*CorePrompt, error) {
	var entity CorePrompt
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	dao.BuildCondition(db, cond)
	if err := db.Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return &entity, nil
}

// GetByAppAndGroup queries CorePrompt by app and group
func (dao *PromptDao) GetByAppAndGroup(ctx context.Context, app, group string) (*CorePrompt, error) {
	var entity CorePrompt
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Where("app = ? AND `group` = ?", app, group).Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetByAppAndGroup fail, app:%s, group:%s, err: %v", app, group, err)
	}
	return &entity, nil
}

// GetByCode queries CorePrompt by business code
func (dao *PromptDao) GetByCode(ctx context.Context, code string) (*CorePrompt, error) {
	var entity CorePrompt
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	if err := db.Where("code = ?", code).Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetByCode fail, code:%s, err: %v", code, err)
	}
	return &entity, nil
}

// GetListByCond queries CorePrompt list by condition
func (dao *PromptDao) GetListByCond(ctx context.Context, cond *PromptCond) (CorePromptList, error) {
	var entityList CorePromptList
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	dao.BuildCondition(db, cond)
	if err := db.Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[PromptDao] GetListByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return entityList, nil
}

// GetPageListByCond queries paginated CorePrompt list by condition with total count
func (dao *PromptDao) GetPageListByCond(ctx context.Context, cond *PromptCond) (CorePromptList, int64, error) {
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
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

// CountByCond counts CorePrompt records by condition
func (dao *PromptDao) CountByCond(ctx context.Context, cond *PromptCond) (int64, error) {
	db := dao.db.WithContext(ctx).Model(&CorePrompt{})
	dao.BuildCondition(db, cond)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("[PromptDao] CountByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return count, nil
}

// BuildCondition builds WHERE condition chain for CorePrompt queries
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
		db.Where(fmt.Sprintf("%s.`group` = ?", tn), cond.Group)
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

// NewPromptVersionDao creates a PromptVersionDao instance with the given db connection
func NewPromptVersionDao(db *gorm.DB) *PromptVersionDao {
	return &PromptVersionDao{db: db}
}

// TableName returns the table name that PromptVersionDao operates on
func (dao *PromptVersionDao) TableName() string {
	return TableNameCorePromptVersion
}

// DB returns the underlying gorm.DB instance
func (dao *PromptVersionDao) DB() *gorm.DB {
	return dao.db
}

// WithTx returns a new PromptVersionDao that uses the given transaction db
func (dao *PromptVersionDao) WithTx(tx *gorm.DB) *PromptVersionDao {
	return &PromptVersionDao{db: tx}
}

// Create inserts a new CorePromptVersion record
func (dao *PromptVersionDao) Create(ctx context.Context, entity *CorePromptVersion) error {
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if err := db.Create(entity).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] Create fail, err: %v", err)
	}
	return nil
}

// CreateBatch batch inserts CorePromptVersion records, empty list returns nil
func (dao *PromptVersionDao) CreateBatch(ctx context.Context, entityList CorePromptVersionList) error {
	if len(entityList) == 0 {
		return nil
	}
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if err := db.Create(entityList).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] CreateBatch fail, err: %v", err)
	}
	return nil
}

// GetByID queries a single CorePromptVersion by primary key
func (dao *PromptVersionDao) GetByID(ctx context.Context, id uint) (*CorePromptVersion, error) {
	var entity CorePromptVersion
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if err := db.Where("id = ?", id).Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptVersionDao] GetByID fail, id:%d, err: %v", id, err)
	}
	return &entity, nil
}

// GetByIDs queries CorePromptVersion records by primary key list, empty list returns nil
func (dao *PromptVersionDao) GetByIDs(ctx context.Context, ids []uint) (CorePromptVersionList, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var entityList CorePromptVersionList
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if err := db.Where("id IN ?", ids).Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[PromptVersionDao] GetByIDs fail, err: %v", err)
	}
	return entityList, nil
}

// UpdateByID updates CorePromptVersion non-zero fields by primary key (struct mapping)
func (dao *PromptVersionDao) UpdateByID(ctx context.Context, id uint, entity *CorePromptVersion) error {
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if err := db.Where("id = ?", id).Updates(entity).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] UpdateByID fail, id:%d, err: %v", id, err)
	}
	return nil
}

// UpdateMap updates CorePromptVersion specified fields by primary key (map mapping)
func (dao *PromptVersionDao) UpdateMap(ctx context.Context, id uint, updateMap map[string]interface{}) error {
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if err := db.Where("id = ?", id).Updates(updateMap).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] UpdateMap fail, id:%d, err: %v", id, err)
	}
	return nil
}

// UpdateMapBatch batch updates CorePromptVersion specified fields by condition
func (dao *PromptVersionDao) UpdateMapBatch(ctx context.Context, cond *PromptVersionCond, updateMap map[string]interface{}) error {
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	dao.BuildCondition(db, cond)
	if err := db.Updates(updateMap).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] UpdateMapBatch fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return nil
}

// SaveBatchOnConflict batch saves CorePromptVersion records, upserting on conflict
func (dao *PromptVersionDao) SaveBatchOnConflict(ctx context.Context, entityList CorePromptVersionList, conflictColumns ...string) error {
	if len(entityList) == 0 {
		return nil
	}
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if len(conflictColumns) > 0 {
		cols := make([]clause.Column, 0, len(conflictColumns))
		for _, c := range conflictColumns {
			cols = append(cols, clause.Column{Name: c})
		}
		db = db.Clauses(clause.OnConflict{Columns: cols, UpdateAll: true})
	}
	if err := db.Create(entityList).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] SaveBatchOnConflict fail, err: %v", err)
	}
	return nil
}

// Delete soft-deletes CorePromptVersion by primary key (sets deleted_at)
func (dao *PromptVersionDao) Delete(ctx context.Context, id uint) error {
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if err := db.Where("id = ?", id).Updates(map[string]interface{}{"deleted_at": time.Now()}).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] Delete fail, id:%d, err: %v", id, err)
	}
	return nil
}

// DeleteByIDs soft-deletes CorePromptVersion by primary key list (sets deleted_at)
func (dao *PromptVersionDao) DeleteByIDs(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if err := db.Where("id IN ?", ids).Updates(map[string]interface{}{"deleted_at": now}).Error; err != nil {
		return fmt.Errorf("[PromptVersionDao] DeleteByIDs fail, err: %v", err)
	}
	return nil
}

// GetByCond queries a single CorePromptVersion by condition
func (dao *PromptVersionDao) GetByCond(ctx context.Context, cond *PromptVersionCond) (*CorePromptVersion, error) {
	var entity CorePromptVersion
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	dao.BuildCondition(db, cond)
	if err := db.Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[PromptVersionDao] GetByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return &entity, nil
}

// GetListByCond queries CorePromptVersion list by condition
func (dao *PromptVersionDao) GetListByCond(ctx context.Context, cond *PromptVersionCond) (CorePromptVersionList, error) {
	var entityList CorePromptVersionList
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	dao.BuildCondition(db, cond)
	if err := db.Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[PromptVersionDao] GetListByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return entityList, nil
}

// GetPageListByCond queries paginated CorePromptVersion list by condition with total count
func (dao *PromptVersionDao) GetPageListByCond(ctx context.Context, cond *PromptVersionCond) (CorePromptVersionList, int64, error) {
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
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

// GetListByPromptID queries all versions under a prompt template by promptID, 0 returns nil
func (dao *PromptVersionDao) GetListByPromptID(ctx context.Context, promptID uint) (CorePromptVersionList, error) {
	if promptID == 0 {
		return nil, nil
	}
	var entityList CorePromptVersionList
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	if err := db.Where("prompt_id = ?", promptID).Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[PromptVersionDao] GetListByPromptID fail, promptID:%d, err: %v", promptID, err)
	}
	return entityList, nil
}

// CountByCond counts CorePromptVersion records by condition
func (dao *PromptVersionDao) CountByCond(ctx context.Context, cond *PromptVersionCond) (int64, error) {
	db := dao.db.WithContext(ctx).Model(&CorePromptVersion{})
	dao.BuildCondition(db, cond)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("[PromptVersionDao] CountByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return count, nil
}

// BuildCondition builds WHERE condition chain for CorePromptVersion queries
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
