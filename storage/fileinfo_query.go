package storage

import (
	"fmt"

	"gorm.io/gorm"
)

// TODO 实验性质，请基于严谨测试后使用

// FileQuery 封装 GORM 链式查询，风格类似 MyBatis-Plus 的 Wrapper
type FileQuery struct {
	db *gorm.DB
}

// NewFileQuery 创建一个新的 FileQuery 实例
func NewFileQuery(db *gorm.DB) *FileQuery {
	return &FileQuery{db: db.Model(&FileInfo{})}
}

// ---------- 通用条件方法 ----------

// Eq 等于条件
func (q *FileQuery) Eq(column string, value any) *FileQuery {
	if value != nil && value != "" {
		q.db = q.db.Where(fmt.Sprintf("%s = ?", column), value)
	}
	return q
}

// Ne 不等于
func (q *FileQuery) Ne(column string, value any) *FileQuery {
	if value != nil && value != "" {
		q.db = q.db.Where(fmt.Sprintf("%s <> ?", column), value)
	}
	return q
}

// Gt 大于
func (q *FileQuery) Gt(column string, value any) *FileQuery {
	if value != nil {
		q.db = q.db.Where(fmt.Sprintf("%s > ?", column), value)
	}
	return q
}

// Lt 小于
func (q *FileQuery) Lt(column string, value any) *FileQuery {
	if value != nil {
		q.db = q.db.Where(fmt.Sprintf("%s < ?", column), value)
	}
	return q
}

// Like 模糊匹配
func (q *FileQuery) Like(column string, keyword string) *FileQuery {
	if keyword != "" {
		q.db = q.db.Where(fmt.Sprintf("%s LIKE ?", column), "%"+keyword+"%")
	}
	return q
}

// In 集合匹配
func (q *FileQuery) In(column string, values any) *FileQuery {
	q.db = q.db.Where(fmt.Sprintf("%s IN ?", column), values)
	return q
}

// OrderBy 排序
func (q *FileQuery) OrderBy(column string, desc bool) *FileQuery {
	if column != "" {
		if desc {
			column += " desc"
		}
		q.db = q.db.Order(column)
	}
	return q
}

// LimitOffset 分页
func (q *FileQuery) LimitOffset(limit, offset int) *FileQuery {
	if limit > 0 {
		q.db = q.db.Limit(limit)
	}
	if offset > 0 {
		q.db = q.db.Offset(offset)
	}
	return q
}

// ---------- 业务字段快捷方法 ----------

func (q *FileQuery) Company(companyID uint) *FileQuery {
	if companyID > 0 {
		q.db = q.db.Where("company_id = ?", companyID)
	}
	return q
}

func (q *FileQuery) Uin(uin uint) *FileQuery {
	if uin > 0 {
		q.db = q.db.Where("uin = ?", uin)
	}
	return q
}

func (q *FileQuery) Purpose(purpose string) *FileQuery {
	if purpose != "" {
		q.db = q.db.Where("purpose = ?", purpose)
	}
	return q
}

func (q *FileQuery) Status(status FileStatus) *FileQuery {
	if status != "" {
		q.db = q.db.Where("status = ?", status)
	}
	return q
}

func (q *FileQuery) Hash(hash string) *FileQuery {
	if hash != "" {
		q.db = q.db.Where("hash = ?", hash)
	}
	return q
}

func (q *FileQuery) StoragePath(path string) *FileQuery {
	if path != "" {
		q.db = q.db.Where("path = ?", path)
	}
	return q
}

// ---------- 查询执行 ----------

// First 获取第一条记录
func (q *FileQuery) First() (*FileInfo, error) {
	var file FileInfo
	err := q.db.First(&file).Error
	return &file, err
}

// List 获取多条记录
func (q *FileQuery) List() ([]FileInfo, error) {
	var list []FileInfo
	err := q.db.Find(&list).Error
	return list, err
}

// Count 获取数量
func (q *FileQuery) Count() (int64, error) {
	var count int64
	err := q.db.Count(&count).Error
	return count, err
}
