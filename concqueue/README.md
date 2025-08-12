# concqueue

`concqueue` 是一个基于生产者-消费者模型的并发任务队列，旨在通过控制并发数和任务队列来简化高并发环境下的任务调度。它通过使用 goroutines 来处理任务，同时通过原子操作确保线程安全，支持任务提交、队列关闭及错误统计。

---

## 特性

- **并发控制**：支持多个 worker 协程并发执行任务。
- **任务队列**：队列容量可定制，任务提交会被缓存，直到有空闲 worker 来执行。
- **队列关闭**：支持优雅关闭，不再接受新任务，等待所有任务完成后退出。
- **错误统计**：统计任务执行过程中的错误数量。
- **线程安全**：通过原子操作和 goroutine 的同步机制保证并发安全。

## 核心功能
- `New(workerCount, queueSize int) *Queue`创建并返回一个新的 `Queue` 实例。
  - `workerCount`：启动的工作协程数。
  - `queueSize`：任务队列的最大容量。

---

- `Submit(t Task) error`提交任务到队列。如果队列已关闭，会返回错误。
  - `t`：一个任务函数，接受 `context.Context` 参数并返回 `error`。

---

- `Shutdown() int`主动关闭队列，等待所有任务完成，并返回错误数量。
  - 关闭后，队列将不再接受新任务，并等待所有 worker 完成处理所有任务。 
  - 在 `Shutdown()` 时，`Queue` 会返回任务处理过程中出现的错误数量。


---
## 使用示例
```go
package main

import (
	"context"
	"fmt"
	"log"
	"github.com/morehao/golib/conc/concqueue"
)

func main() {
	// 定义一个简单的任务处理函数
	Handler := func(ctx context.Context, input int) (string, error) {
		if input%2 == 0 {
			return "", fmt.Errorf("输入参数为偶数")
		}
		return fmt.Sprintf("处理结果：%d", input), nil
	}

	// 创建一个新的队列实例，3个 worker，队列大小为 10
	q := concqueue.New(3, 10)

	// 提交 5 个任务
	taskCount := 5
	res := make([]string, taskCount)
	errs := make([]error, taskCount)

	for i := 0; i < taskCount; i++ {
		n := i // 传递索引，避免并发问题
		err := q.Submit(func(ctx context.Context) error {
			currRes, err := Handler(ctx, n)
			if err != nil {
				errs[n] = err
			} else {
				res[n] = currRes
			}
			return err
		})
		if err != nil {
			log.Printf("提交任务失败：%v", err)
		}
	}

	// 等待任务完成并关闭队列
	errCnt := q.Shutdown()

	// 输出结果和错误数量
	log.Printf("错误数量：%d", errCnt)
	for i := 0; i < taskCount; i++ {
		if errs[i] != nil {
			log.Printf("任务 %d 失败: %v", i, errs[i])
		} else {
			log.Printf("任务 %d 成功: %s", i, res[i])
		}
	}
}
```