package stat

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// ==================== 定义测试用的查询类型 ====================

// CompanyQuery 公司统计查询
type CompanyQuery struct {
	CompanyID uint
	BeginAt   time.Time
	EndAt     time.Time
}

func (q CompanyQuery) Validate() error {
	if q.CompanyID == 0 {
		return fmt.Errorf("company_id is required")
	}
	if !q.EndAt.IsZero() && q.EndAt.Before(q.BeginAt) {
		return fmt.Errorf("end_at must be after begin_at")
	}
	return nil
}

func TestStatManager(t *testing.T) {
	// 1. 创建统计管理器
	manager := NewStatManager[CompanyQuery]()

	// 2. 注册统计函数
	manager.Register("user_count", func(ctx context.Context, q CompanyQuery) (MetricValue, error) {
		// 实际场景：这里会查询数据库
		fmt.Printf("统计公司 %d 的用户数\n", q.CompanyID)
		return IntMetric{Value: 100}, nil
	})

	manager.Register("revenue", func(ctx context.Context, q CompanyQuery) (MetricValue, error) {
		// 实际场景：这里会查询订单表
		fmt.Printf("统计公司 %d 的收入\n", q.CompanyID)
		return FloatMetric{Value: 50000.5}, nil
	})

	// 3. 批量注册
	funcs := map[string]StatFunc[CompanyQuery]{
		"order_count": func(ctx context.Context, q CompanyQuery) (MetricValue, error) {
			return IntMetric{Value: 200}, nil
		},
		"active_users": func(ctx context.Context, q CompanyQuery) (MetricValue, error) {
			return IntMetric{Value: 80}, nil
		},
	}
	manager.BatchRegister(funcs)

	// 4. 执行统计
	ctx := context.Background()
	query := CompanyQuery{
		CompanyID: 123,
		BeginAt:   time.Now().AddDate(0, -1, 0),
		EndAt:     time.Now(),
	}

	results, err := manager.Execute(ctx, query)
	if err != nil {
		fmt.Printf("执行失败: %v\n", err)
		return
	}

	// 5. 使用结果
	fmt.Printf("统计完成，共 %d 项指标:\n", len(results))
	for name, value := range results {
		fmt.Printf("  %s: %d\n", name, value.GetIntValue())
	}

	// Output:
	// 统计公司 123 的用户数
	// 统计公司 123 的收入
	// 统计完成，共 4 项指标:
}
