package concqueue

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func Test_Queue(t *testing.T) {
	Handler := func(ctx context.Context, input int) (string, error) {
		fmt.Println(input, "处理中...")
		time.Sleep(time.Second * 2)
		if input%2 == 0 {
			return "", fmt.Errorf("输入参数为偶数")
		}
		res := fmt.Sprintf("处理结果：%d", input)
		return res, nil
	}

	t.Run("NormalTest", func(t *testing.T) {
		start := time.Now()

		// 创建队列，使用5个worker，队列大小为10
		q := New(5, 10)
		taskCount := 5
		res := make([]string, taskCount)
		errs := make([]error, taskCount)

		// 提交任务
		for i := 0; i < taskCount; i++ {
			n := i // 传递索引值给任务，避免并发时数据竞争
			q.Submit(func(ctx context.Context) error {
				currRes, err := Handler(ctx, n)
				if err != nil {
					errs[n] = err
				} else {
					res[n] = currRes
				}
				return err
			})
		}

		// 等待任务完成并关闭队列
		errCnt := q.StopAndWait()

		// 打印并检查错误数量
		t.Logf("错误数量：%d", errCnt)

		// 验证结果
		for i := 0; i < taskCount; i++ {
			if i%2 == 0 {
				if errs[i] == nil {
					t.Errorf("任务 %d 应该返回错误，但没有返回错误", i)
				}
			} else {
				if errs[i] != nil {
					t.Errorf("任务 %d 不应返回错误，但返回了错误: %v", i, errs[i])
				} else if res[i] != fmt.Sprintf("处理结果：%d", i) {
					t.Errorf("任务 %d 结果不正确，期望 %s，但实际是 %s", i, fmt.Sprintf("处理结果：%d", i), res[i])
				}
			}
		}

		// 断言错误数量正确
		var expectedErrCount int32 = 3 // 偶数任务会返回错误
		if errCnt != expectedErrCount {
			t.Errorf("错误数量不符，期望 %d，但实际是 %d", expectedErrCount, errCnt)
		}

		// 打印所有任务完成的时间
		t.Log("所有任务完成，耗时：", time.Since(start))
	})

	t.Run("QueueCloseTest", func(t *testing.T) {
		// 创建队列，使用5个worker，队列大小为10
		q := New(5, 10)

		// 提交任务并停止队列
		q.Submit(func(ctx context.Context) error {
			return nil
		})
		// 停止并关闭队列
		q.StopAndWait()

		// 队列关闭后再提交任务，应该被丢弃
		q.Submit(func(ctx context.Context) error {
			t.Fatal("任务不应提交成功")
			return nil
		})
	})

	t.Run("QueueWithCapacityTest", func(t *testing.T) {
		// 创建队列，使用2个worker，队列大小为2
		q := New(2, 2)

		// 提交超过队列容量的任务
		for i := 0; i < 5; i++ {
			n := i
			q.Submit(func(ctx context.Context) error {
				if n%2 == 0 {
					return fmt.Errorf("任务 %d 出错", n)
				}
				return nil
			})
		}

		// 停止并等待任务完成
		errCnt := q.StopAndWait()

		// 断言错误数量，偶数任务会返回错误
		expectedErrCount := int32(3)
		if errCnt != expectedErrCount {
			t.Errorf("错误数量不符，期望 %d，但实际是 %d", expectedErrCount, errCnt)
		}
	})

	t.Run("EmptyQueueTest", func(t *testing.T) {
		// 创建一个没有任务容量的队列
		q := New(2, 0)

		// 队列没有容量，不会有任务提交
		// 应立即返回，不应该有错误
		if errCount := q.StopAndWait(); errCount != 0 {
			t.Errorf("错误数量不符，期望 0，但实际是 %d", errCount)
		}
	})

	t.Run("TaskTimeoutTest", func(t *testing.T) {
		Handler := func(ctx context.Context, input int) (string, error) {
			select {
			case <-time.After(3 * time.Second): // 增加时间，确保任务可以完成
				return fmt.Sprintf("处理结果：%d", input), nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		q := New(2, 5)
		taskCount := 3
		res := make([]string, taskCount)
		errs := make([]error, taskCount)

		// 提交任务
		for i := 0; i < taskCount; i++ {
			n := i // 避免并发数据竞争
			q.Submit(func(ctx context.Context) error {
				currRes, err := Handler(ctx, n)
				if err != nil {
					errs[n] = err
				} else {
					res[n] = currRes
				}
				return err
			})
		}

		// 等待任务完成并关闭队列
		errCnt := q.StopAndWait()

		// 断言任务超时错误
		for i := 0; i < taskCount; i++ {
			if errs[i] != nil {
				t.Errorf("任务 %d 出现错误: %v", i, errs[i])
			} else if res[i] != fmt.Sprintf("处理结果：%d", i) {
				t.Errorf("任务 %d 结果不正确，期望 %s，但实际是 %s", i, fmt.Sprintf("处理结果：%d", i), res[i])
			}
		}

		// 断言错误数量
		if errCnt != 0 {
			t.Errorf("错误数量不符，期望 0，但实际是 %d", errCnt)
		}
	})

}
