// Command c_remote 演示「形态 C：远程 Worker」——执行节点只连中心、不直接连库。
//
// 拓扑：
//
//	         （中心侧进程）                          （远端执行进程）
//	manager（落库 + 维护循环）                    source/remote（HTTP 三段）
//	        │                                            │
//	        ├── source/db  ← DB Source                  │ Fetch/Renew/Save/Release
//	        └── center ──── wire 协议 ──── HTTP ────────┘
//	                        ▲
//	               HTTP 绑定由接入方用 gin/httptools 自行挂载
//	               （yg-go 本期不提供 ginasync.RegisterCenter）
//
// 本示例在单进程内用 net/http 把 center 的 4 个方法挂到一个 http.Server（httptest）上，
// 让 source/remote 连过去跑一轮 Fetch → 执行 → Callback，验证远程形态端到端可通。
//
// 运行（sqlite 内存库，无需外部依赖）：
//
//	go run ./asynctask/example/c_remote
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/center"
	"github.com/ygpkg/yg-go/asynctask/manager"
	"github.com/ygpkg/yg-go/asynctask/source/remote"
	"github.com/ygpkg/yg-go/asynctask/wire"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// EmailPayload 载荷示例。
type EmailPayload struct {
	To string `json:"to"`
}

const taskType = "email:remote"

// mountCenter 用标准 net/http 把 center 的 4 个方法挂到路由（接入方职责，yg-go 不提供绑定）。
// 真实项目里换成 gin/httptools；端点路径一律取 wire.PathXxx 常量，避免两端漂移。
func mountCenter(c *center.Center) http.Handler {
	mux := http.NewServeMux()

	decode := func(r *http.Request, v any) error {
		return json.NewDecoder(r.Body).Decode(v)
	}
	writeJSON := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if v != nil {
			_ = json.NewEncoder(w).Encode(v)
		}
	}

	mux.HandleFunc(wire.PathFetch, func(w http.ResponseWriter, r *http.Request) {
		var req wire.FetchRequest
		if err := decode(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := c.Fetch(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc(wire.PathHeartbeat, func(w http.ResponseWriter, r *http.Request) {
		var req wire.HeartbeatRequest
		if err := decode(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := c.Heartbeat(r.Context(), req); err != nil {
			http.Error(w, err.Error(), http.StatusConflict) // 非持有者 ErrLeaseLost 等
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc(wire.PathCallback, func(w http.ResponseWriter, r *http.Request) {
		var req wire.CallbackRequest
		if err := decode(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := c.Callback(r.Context(), req); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc(wire.PathRelease, func(w http.ResponseWriter, r *http.Request) {
		var req wire.ReleaseRequest
		if err := decode(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := c.Release(r.Context(), req); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// ===== 中心侧进程 =====
	db, err := gorm.Open(sqlite.Open("file:example_c?mode=memory&cache=shared"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	mgr, err := manager.New(&manager.Config{LeaseTTL: 60 * time.Second}, db)
	if err != nil {
		log.Fatalf("manager.New: %v", err)
	}
	defer mgr.Close()
	if err := mgr.Start(ctx); err != nil { // 远端 worker 崩溃后靠维护循环判死
		log.Fatalf("manager.Start: %v", err)
	}

	c, err := center.New(mgr)
	if err != nil {
		log.Fatalf("center.New: %v", err)
	}

	// 这里用 httptest 起一个真实 HTTP server 代替生产里的 gin 路由。
	srv := httptest.NewServer(mountCenter(c))
	defer srv.Close()

	// 提交一个任务到中心（中心进程通常由业务侧提交）。
	payload, _ := json.Marshal(EmailPayload{To: "r@example.com"})
	if _, _, err := mgr.Submit(ctx, &asynctask.AsyncTaskSpec{Type: taskType, Payload: payload}); err != nil {
		log.Fatalf("submit: %v", err)
	}

	// ===== 远端执行进程（只连中心，不连库）=====
	src, err := remote.New(remote.Config{BaseURL: srv.URL, WorkerID: "w-1"})
	if err != nil {
		log.Fatalf("remote.New: %v", err)
	}
	w := worker.New(src, &worker.WorkerConfig{Concurrency: 2, LeaseTTL: 60 * time.Second})
	if err := w.Register(taskType, worker.HandlerFunc(func(ctx context.Context, t *asynctask.AsyncTask) error {
		var p EmailPayload
		if err := t.Bind(&p); err != nil {
			return worker.ErrNoRetry
		}
		fmt.Printf("[remote execute] send email to=%s attempt=%d\n", p.To, t.Attempt)
		return nil
	})); err != nil {
		log.Fatalf("register: %v", err)
	}
	if err := w.Start(ctx); err != nil {
		log.Fatalf("worker.Start: %v", err)
	}
	defer w.Shutdown(context.Background())

	time.Sleep(2 * time.Second)
	fmt.Println("[main] done")
}
