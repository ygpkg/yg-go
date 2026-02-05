package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

type TaskCond struct {
	CompanyID   uint
	Uin         uint
	ID          uint
	IDs         []uint
	WorkerID    string
	TaskType    string
	TaskStatus  TaskStatus
	OrderBy     []string
	OrCondition OrCondition
	Offset      int
	Limit       int
	IsDelete    bool
}

type OrCondition struct {
	Conditions []string
	Args       []any
}

type TaskDao struct {
	db *gorm.DB
}

func NewTaskDao(db *gorm.DB) *TaskDao {
	return &TaskDao{db: db}
}

func (dao *TaskDao) TableName() string {
	return TableNameCoreTask
}

func (dao *TaskDao) Insert(ctx context.Context, entity *TaskEntity) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Create(entity).Error; err != nil {
		return fmt.Errorf("[TaskDao] Insert fail, entity:%s, err: %v", logs.JSON(entity), err)
	}
	return nil
}

func (dao *TaskDao) BatchInsert(ctx context.Context, entityList TaskList) error {
	if len(entityList) == 0 {
		return fmt.Errorf("[TaskDao] BatchInsert fail, entityList is empty")
	}

	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Create(entityList).Error; err != nil {
		return fmt.Errorf("[TaskDao] BatchInsert fail, entityList:%s, err: %v", logs.JSON(entityList), err)
	}
	return nil
}

func (dao *TaskDao) UpdateByID(ctx context.Context, id uint, entity *TaskEntity) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Updates(entity).Error; err != nil {
		return fmt.Errorf("[TaskDao] UpdateByID fail, id:%d, entity:%s, err: %v", id, logs.JSON(entity), err)
	}
	return nil
}

func (dao *TaskDao) UpdateMap(ctx context.Context, id uint, updateMap map[string]interface{}) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Updates(updateMap).Error; err != nil {
		return fmt.Errorf("[TaskDao] UpdateMap fail, id:%d, updateMap:%s, err: %v", id, logs.JSON(updateMap), err)
	}
	return nil
}

func (dao *TaskDao) Delete(ctx context.Context, id uint) error {
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	updatedField := map[string]interface{}{
		"deleted_at": time.Now(),
	}
	if err := db.Where("id = ?", id).Updates(updatedField).Error; err != nil {
		return fmt.Errorf("[TaskDao] Delete fail, id:%d, err: %v", id, err)
	}
	return nil
}

func (dao *TaskDao) GetByID(ctx context.Context, id uint) (*TaskEntity, error) {
	var entity TaskEntity
	db := dao.db.WithContext(ctx).Table(dao.TableName())
	if err := db.Where("id = ?", id).Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[TaskDao] GetByID fail, id:%d, err: %v", id, err)
	}
	return &entity, nil
}

func (dao *TaskDao) GetByCond(ctx context.Context, cond *TaskCond) (*TaskEntity, error) {
	var entity TaskEntity
	db := dao.db.WithContext(ctx).Table(dao.TableName())

	dao.BuildCondition(db, cond)

	if err := db.Find(&entity).Error; err != nil {
		return nil, fmt.Errorf("[TaskDao] GetByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return &entity, nil
}

func (dao *TaskDao) GetListByCond(ctx context.Context, cond *TaskCond) (TaskList, error) {
	var entityList TaskList
	db := dao.db.WithContext(ctx).Table(dao.TableName())

	dao.BuildCondition(db, cond)

	if err := db.Find(&entityList).Error; err != nil {
		return nil, fmt.Errorf("[TaskDao] GetListByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return entityList, nil
}

func (dao *TaskDao) GetPageListByCond(ctx context.Context, cond *TaskCond) (TaskList, int64, error) {
	db := dao.db.WithContext(ctx).Model(&TaskEntity{}).Table(dao.TableName())

	dao.BuildCondition(db, cond)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("[TaskDao] GetPageListByCond count fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	if cond.Limit > 0 {
		db.Limit(cond.Limit)
	}
	if cond.Offset > 0 {
		db.Offset(cond.Offset)
	}
	var entityList TaskList
	if err := db.Find(&entityList).Error; err != nil {
		return nil, 0, fmt.Errorf("[TaskDao] GetPageListByCond find fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return entityList, count, nil
}

func (dao *TaskDao) CountByCond(ctx context.Context, cond *TaskCond) (int64, error) {
	db := dao.db.WithContext(ctx).Model(&TaskEntity{}).Table(dao.TableName())

	dao.BuildCondition(db, cond)
	var count int64
	if err := db.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("[TaskDao] CountByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return count, nil
}

// 平均耗时统计(秒)
func (dao *TaskDao) AvgCostTimeByCond(ctx context.Context, cond *TaskCond) (float64, error) {
	db := dao.db.WithContext(ctx).Model(&TaskEntity{}).Table(dao.TableName())
	dao.BuildCondition(db, cond)

	var avgCost float64
	if err := db.Select("COALESCE(AVG(cost), 0)").Scan(&avgCost).Error; err != nil {
		return 0, fmt.Errorf("[TaskDao] AvgCostTimeByCond fail, cond:%s, err: %v", logs.JSON(cond), err)
	}
	return avgCost, nil
}

func (dao *TaskDao) BuildCondition(db *gorm.DB, cond *TaskCond) {
	if cond.CompanyID > 0 {
		query := fmt.Sprintf("%s.company_id = ?", dao.TableName())
		db.Where(query, cond.CompanyID)
	}
	if cond.Uin > 0 {
		query := fmt.Sprintf("%s.uin = ?", dao.TableName())
		db.Where(query, cond.Uin)
	}
	if cond.WorkerID != "" {
		query := fmt.Sprintf("%s.worker_id = ?", dao.TableName())
		db.Where(query, cond.WorkerID)
	}
	if cond.ID > 0 {
		query := fmt.Sprintf("%s.id = ?", dao.TableName())
		db.Where(query, cond.ID)
	}
	if len(cond.IDs) > 0 {
		query := fmt.Sprintf("%s.id in ?", dao.TableName())
		db.Where(query, cond.IDs)
	}
	if cond.TaskType != "" {
		query := fmt.Sprintf("%s.task_type = ?", dao.TableName())
		db.Where(query, cond.TaskType)
	}
	if cond.TaskStatus != "" {
		query := fmt.Sprintf("%s.task_status = ?", dao.TableName())
		db.Where(query, cond.TaskStatus)
	}
	if cond.IsDelete {
		db.Unscoped()
	}
	if len(cond.OrderBy) > 0 {
		db.Order(strings.Join(cond.OrderBy, ","))
	}

	if len(cond.OrCondition.Conditions) > 0 {
		if len(cond.OrCondition.Args) == len(cond.OrCondition.Conditions) {
			whereClause := strings.Join(cond.OrCondition.Conditions, " OR ")
			db = db.Where("("+whereClause+")", cond.OrCondition.Args...)
		}
	}
}
