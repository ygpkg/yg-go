package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

// activeStatusesSQL 是「活跃期」的状态谓词，与 DDL 里 uk_core_async_task_uniq_active
// 的 WHERE 子句（PG/SQLite 部分唯一索引、MySQL 生成列条件）保持逐字一致。
const activeStatusesSQL = "status IN ('pending','running')"

// Store 是管理面的只读存储 + Cleanup。写操作走 Manager。
type Store interface {
	GetAsyncTask(ctx context.Context, id string) (*asynctask.AsyncTask, error)
	GetAsyncTaskWithRuns(ctx context.Context, id string) (*asynctask.AsyncTask, []*asynctask.AsyncTaskRun, error)
	ListAsyncTask(ctx context.Context, cond *asynctask.AsyncTaskCond) ([]*asynctask.AsyncTask, int64, error)
	ListRun(ctx context.Context, cond *asynctask.AsyncTaskRunCond) ([]*asynctask.AsyncTaskRun, int64, error)
	GetRun(ctx context.Context, id string) (*asynctask.AsyncTaskRun, error)
	// GetActiveByUniqKey 按幂等键取「活跃期」那一行（pending/running），不存在返回 ErrAsyncTaskNotFound。
	GetActiveByUniqKey(ctx context.Context, uniqKey string) (*asynctask.AsyncTask, error)
	// GetLatestBySubject 按业务主体取最近一条（taskType 为空表示不限类型）。
	GetLatestBySubject(ctx context.Context, kind, id, taskType string) (*asynctask.AsyncTask, error)
	// StatsByStatus 按条件分组统计各状态行数（cond 为 nil 表示全表）。
	StatsByStatus(ctx context.Context, cond *asynctask.AsyncTaskCond) (map[string]int64, error)
	Cleanup(ctx context.Context, before time.Time, batch int) (tasks int64, runs int64, err error)
}

// GetAsyncTask 按 ID 取任务，不存在返回 ErrAsyncTaskNotFound。
func (s *store) GetAsyncTask(ctx context.Context, id string) (*asynctask.AsyncTask, error) {
	row, err := s.loadAsyncTaskRow(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, asynctask.ErrAsyncTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return asyncTaskRowToDomain(row), nil
}

// GetAsyncTaskWithRuns 取任务及其跨重放的全部尝试记录（按 generation, attempt 升序）。
// 任务不存在返回 ErrAsyncTaskNotFound。
func (s *store) GetAsyncTaskWithRuns(ctx context.Context, id string) (*asynctask.AsyncTask, []*asynctask.AsyncTaskRun, error) {
	asyncTask, err := s.GetAsyncTask(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	var rows []CoreAsyncTaskRun
	if err := s.db.WithContext(ctx).Where("async_task_id = ?", id).
		Order("generation ASC, attempt ASC").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	return asyncTask, runRowsToDomain(rows), nil
}

// GetActiveByUniqKey 按幂等键取活跃期（pending/running）那一行。
//
// 命中 DDL 的 uk_core_async_task_uniq_active 唯一索引（PG/SQLite 部分唯一索引、
// MySQL 生成列唯一键），所以最多返回一行；不存在（无行或只剩终态行）返回 ErrAsyncTaskNotFound。
// Submit 的冲突回查用它：只有「活跃行」才算幂等命中，终态行一律视为可重建。
func (s *store) GetActiveByUniqKey(ctx context.Context, uniqKey string) (*asynctask.AsyncTask, error) {
	if uniqKey == "" {
		return nil, asynctask.ErrAsyncTaskNotFound
	}
	var row CoreAsyncTask
	err := s.db.WithContext(ctx).
		Where("uniq_key = ? AND "+activeStatusesSQL, uniqKey).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, asynctask.ErrAsyncTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return asyncTaskRowToDomain(&row), nil
}

// GetLatestBySubject 按业务主体取最近一条任务。
//
// 带 taskType 时命中 idx_core_async_task_subject（subject_kind, subject_id, task_type, created_at DESC），
// 排序由索引提供、不产生 filesort；不带 taskType 时只能用到索引前缀 + filesort，
// 但结果集已被主体限定在几十行内，可接受。
func (s *store) GetLatestBySubject(ctx context.Context, kind, id, taskType string) (*asynctask.AsyncTask, error) {
	var row CoreAsyncTask
	q := s.db.WithContext(ctx).
		Where("subject_kind = ? AND subject_id = ?", kind, id)
	if taskType != "" {
		q = q.Where("task_type = ?", taskType)
	}
	err := q.Order("created_at DESC, id DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, asynctask.ErrAsyncTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return asyncTaskRowToDomain(&row), nil
}

// StatsByStatus 按 cond 过滤后分组统计各状态行数。cond 为 nil 表示不过滤。
func (s *store) StatsByStatus(ctx context.Context, cond *asynctask.AsyncTaskCond) (map[string]int64, error) {
	if cond == nil {
		cond = &asynctask.AsyncTaskCond{}
	}
	var rows []struct {
		Status string
		Cnt    int64
	}
	q := s.applyTaskCond(s.db.WithContext(ctx).Model(&CoreAsyncTask{}), cond)
	if err := q.Select("status, count(*) AS cnt").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Status] = r.Cnt
	}
	return out, nil
}

// ListAsyncTask 分页查询任务，返回当页与总数。条件缺省即不过滤；PageSize<=0 默认 20，上限 200。
//
// cond.AfterCreatedAt 非 nil 时进入 keyset 分页：排序固定为 created_at（方向由 OrderBy 的
// created_at 部分决定，默认 DESC），返回「游标之后」的行；此时 Offset 仍会生效，但通常应为 0。
func (s *store) ListAsyncTask(ctx context.Context, cond *asynctask.AsyncTaskCond) ([]*asynctask.AsyncTask, int64, error) {
	if cond == nil {
		cond = &asynctask.AsyncTaskCond{}
	}
	page, size := normPage(cond.Page, cond.PageSize)
	q := s.applyTaskCond(s.db.WithContext(ctx).Model(&CoreAsyncTask{}), cond)
	q = s.applyKeyset(q, cond)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []CoreAsyncTask
	if err := q.Order(keysetOrder(cond.OrderBy, cond.AfterCreatedAt != nil)).
		Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return asyncTaskRowsToDomain(rows), total, nil
}

// applyTaskCond 把 AsyncTaskCond 的条件（除分页/排序外的过滤项）挂到查询上。
// 列表、统计共用这一份，避免两处口径漂移（例如租户过滤只允许出现 company_id 列）。
func (s *store) applyTaskCond(q *gorm.DB, cond *asynctask.AsyncTaskCond) *gorm.DB {
	if cond.Type != "" {
		q = q.Where("task_type = ?", cond.Type)
	}
	if cond.Status != "" {
		q = q.Where("status = ?", string(cond.Status))
	}
	if cond.BizType != "" {
		q = q.Where("biz_type = ?", cond.BizType)
	}
	if cond.BizID != "" {
		q = q.Where("biz_id = ?", cond.BizID)
	}
	if cond.ChainID != "" {
		q = q.Where("chain_id = ?", cond.ChainID)
	}
	// CompanyID 用指针：0 是合法租户，不能用零值判断「未设置」。
	if cond.CompanyID != nil {
		q = q.Where("company_id = ?", *cond.CompanyID)
	}
	if cond.SubjectKind != "" {
		q = q.Where("subject_kind = ?", cond.SubjectKind)
	}
	if cond.SubjectID != "" {
		q = q.Where("subject_id = ?", cond.SubjectID)
	}
	if cond.UniqKey != "" {
		q = q.Where("uniq_key = ?", cond.UniqKey)
	}
	if cond.ActiveOnly {
		q = q.Where(activeStatusesSQL)
	}
	if cond.CreatedAfter != nil {
		q = q.Where("created_at >= ?", *cond.CreatedAfter)
	}
	if cond.CreatedBefore != nil {
		q = q.Where("created_at <= ?", *cond.CreatedBefore)
	}
	// Labels 精确匹配（AND）：按方言拼 JSON 取值条件。
	for k, v := range cond.Labels {
		if k == "" {
			continue
		}
		col, arg := s.dialect.jsonExtract("labels", k, v)
		q = q.Where(col, arg...)
	}
	return q
}

// applyKeyset 追加 keyset 游标条件（仅 created_at 游标；AfterCreatedAt 为 nil 时不生效）。
//
// 用行值比较 (created_at, id) </> (?, ?)：三方言都支持，且能命中
// (created_at, id) 形状的索引顺序，深页不再随 Offset 变慢。
func (s *store) applyKeyset(q *gorm.DB, cond *asynctask.AsyncTaskCond) *gorm.DB {
	if cond.AfterCreatedAt == nil {
		return q
	}
	if keysetAscending(cond.OrderBy) {
		return q.Where("(created_at, id) > (?, ?)", *cond.AfterCreatedAt, cond.AfterID)
	}
	return q.Where("(created_at, id) < (?, ?)", *cond.AfterCreatedAt, cond.AfterID)
}

// keysetOrder 返回列表排序子句。
//
// keyset 生效时排序被强制为 created_at 方向（否则游标的行值比较与 ORDER BY 会错位）：
// 显式 created_at asc 时升序，其余（含默认）降序；id 的次序与游标比较方向保持一致。
func keysetOrder(orderBy string, keyset bool) string {
	if !keyset {
		return orderByClause(orderBy)
	}
	if keysetAscending(orderBy) {
		return "created_at ASC, id ASC"
	}
	return "created_at DESC, id DESC"
}

// keysetAscending 判断 keyset 游标方向：只有显式 created_at asc 才升序，其余按默认降序。
func keysetAscending(orderBy string) bool {
	return strings.HasPrefix(orderByClause(orderBy), "created_at ASC")
}

// orderByClause 把 AsyncTaskCond.OrderBy 白名单化；默认 created_at desc。
func orderByClause(ob string) string {
	switch strings.TrimSpace(strings.ToLower(ob)) {
	case "created_at asc", "created_at":
		return "created_at ASC, id ASC"
	case "created_at desc":
		return "created_at DESC, id DESC"
	case "updated_at asc", "updated_at":
		return "updated_at ASC, id DESC"
	case "updated_at desc":
		return "updated_at DESC, id DESC"
	case "priority asc", "priority":
		return "priority ASC, id DESC"
	case "priority desc":
		return "priority DESC, id DESC"
	case "status asc", "status":
		return "status ASC, id DESC"
	case "status desc":
		return "status DESC, id DESC"
	default:
		return "created_at DESC, id DESC"
	}
}

// ListRun 分页查询尝试记录。条件缺省即不过滤；PageSize<=0 默认 20，上限 200。
func (s *store) ListRun(ctx context.Context, cond *asynctask.AsyncTaskRunCond) ([]*asynctask.AsyncTaskRun, int64, error) {
	page, size := normPage(cond.Page, cond.PageSize)
	q := s.db.WithContext(ctx).Model(&CoreAsyncTaskRun{})
	if cond.AsyncTaskID != "" {
		q = q.Where("async_task_id = ?", cond.AsyncTaskID)
	}
	if cond.Generation != 0 {
		q = q.Where("generation = ?", cond.Generation)
	}
	if cond.Status != "" {
		q = q.Where("status = ?", string(cond.Status))
	}
	if cond.Failure != "" {
		q = q.Where("failure_kind = ?", string(cond.Failure))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []CoreAsyncTaskRun
	if err := q.Order("created_at DESC, id DESC").
		Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return runRowsToDomain(rows), total, nil
}

// GetRun 按 ID 取尝试记录，不存在返回 ErrRunNotFound。
func (s *store) GetRun(ctx context.Context, id string) (*asynctask.AsyncTaskRun, error) {
	var row CoreAsyncTaskRun
	err := s.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, asynctask.ErrRunNotFound
	}
	if err != nil {
		return nil, err
	}
	return runRowToDomain(&row), nil
}

// normPage 归一化分页参数。
func normPage(page, size int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	return page, size
}
