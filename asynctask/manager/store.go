package manager

import "github.com/ygpkg/yg-go/asynctask/source/db"

// Store 是管理面的只读存储 + Cleanup 接口（供外部/HTTP 适配层读取任务与尝试记录）。
//
// 直接复用 source/db 的 Store 接口定义，避免两处各维护一份方法集导致漂移。
// 写操作走 Manager 的 Submit/Cancel/Replay。
type Store = db.Store
