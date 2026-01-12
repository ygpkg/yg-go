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
	manager := NewStatManager[CompanyQuery, uint]()

	// 2. 注册统计函数
	manager.Register("user_count", func(ctx context.Context, q CompanyQuery) (MetricValue, map[uint]MetricValue, error) {
		// 实际场景：这里会查询数据库
		fmt.Printf("统计公司 %d 的用户数\n", q.CompanyID)
		return IntMetric{Value: 100}, nil, nil
	})

	manager.Register("revenue", func(ctx context.Context, q CompanyQuery) (MetricValue, map[uint]MetricValue, error) {
		// 实际场景：这里会查询订单表
		fmt.Printf("统计公司 %d 的收入\n", q.CompanyID)
		groupStat := map[uint]MetricValue{
			1: FloatMetric{Value: 30000.0},
		}
		return nil, groupStat, nil
	})

	// 3. 批量注册
	funcs := map[string]StatFunc[CompanyQuery, uint]{
		"order_count": func(ctx context.Context, q CompanyQuery) (MetricValue, map[uint]MetricValue, error) {
			return IntMetric{Value: 200}, nil, nil
		},
		"active_users": func(ctx context.Context, q CompanyQuery) (MetricValue, map[uint]MetricValue, error) {
			return IntMetric{Value: 80}, nil, nil
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
	for name, result := range results {
		if result.IsSuccess() {
			if result.IsGrouped {
				fmt.Printf("  %s (分组): %d 个分组\n", name, len(result.GetGroupedValue()))
			} else {
				fmt.Printf("  %s (单值): %v\n", name, result.GetSingleValue())
			}
		} else {
			fmt.Printf("  %s: 执行失败 - %v\n", name, result.Error)
		}
	}
}

// TestStatManagerValidation 测试参数验证
func TestStatManagerValidation(t *testing.T) {
	manager := NewStatManager[CompanyQuery, uint]()

	manager.Register("test", func(ctx context.Context, q CompanyQuery) (MetricValue, map[uint]MetricValue, error) {
		return IntMetric{Value: 1}, nil, nil
	})

	// 测试无效查询
	ctx := context.Background()
	invalidQuery := CompanyQuery{CompanyID: 0} // CompanyID 不能为 0

	_, err := manager.Execute(ctx, invalidQuery)
	if err == nil {
		t.Error("期望验证失败，但成功了")
	}
	fmt.Printf("验证失败（符合预期）: %v\n", err)
}

// TestStatManagerError 测试统计函数失败的情况
func TestStatManagerError(t *testing.T) {
	manager := NewStatManager[CompanyQuery, uint]()

	// 注册会失败的函数
	manager.Register("error_stat", func(ctx context.Context, q CompanyQuery) (MetricValue, map[uint]MetricValue, error) {
		return nil, nil, fmt.Errorf("模拟错误")
	})

	// 注册正常的函数
	manager.Register("normal_stat", func(ctx context.Context, q CompanyQuery) (MetricValue, map[uint]MetricValue, error) {
		return IntMetric{Value: 100}, nil, nil
	})

	ctx := context.Background()
	query := CompanyQuery{CompanyID: 1}

	results, err := manager.Execute(ctx, query)
	if err == nil {
		t.Error("期望执行失败，但成功了")
	}

	// 检查部分成功的结果
	if errorResult, ok := results["error_stat"]; ok {
		if errorResult.IsSuccess() {
			t.Error("error_stat 应该失败")
		}
		fmt.Printf("失败的统计: %s - %v\n", errorResult.Name, errorResult.Error)
	}

	if normalResult, ok := results["normal_stat"]; ok {
		if !normalResult.IsSuccess() {
			t.Error("normal_stat 应该成功")
		}
		fmt.Printf("成功的统计: %s - %v\n", normalResult.Name, normalResult.GetSingleValue())
	}
}

// TestStatManagerInvalidReturn 测试返回值验证
func TestStatManagerInvalidReturn(t *testing.T) {
	manager := NewStatManager[CompanyQuery, uint]()

	// 注册同时返回单值和分组值的函数（错误用法）
	manager.Register("invalid_stat", func(ctx context.Context, q CompanyQuery) (MetricValue, map[uint]MetricValue, error) {
		return IntMetric{Value: 100}, map[uint]MetricValue{1: IntMetric{Value: 200}}, nil
	})

	ctx := context.Background()
	query := CompanyQuery{CompanyID: 1}

	results, err := manager.Execute(ctx, query)
	if err == nil {
		t.Error("期望执行失败，但成功了")
	}

	if result, ok := results["invalid_stat"]; ok {
		if result.IsSuccess() {
			t.Error("invalid_stat 应该失败")
		}
		fmt.Printf("返回值验证失败（符合预期）: %v\n", result.Error)
	}
}
