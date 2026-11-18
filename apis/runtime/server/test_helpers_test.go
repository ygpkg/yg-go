package server

import (
	"fmt"
	"io"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// 本文件集中放 server 包测试共用的断言与请求 helper。
// 单独成文件，避免改动某个用例文件时让其他测试文件的 helper 一起消失（假红）。

// routeKey 是 Routes() 快照中参与比较的三元组。
// Source 带文件行号、跨用例不稳定，只在 TestRoutes / TestDuplicateRoutePanics 里单独断言。
type routeKey struct {
	Method string
	Path   string
	Prefix string
}

// sortRouteKeys 按 Path → Method → Prefix 排序，让快照比较与注册顺序解耦。
func sortRouteKeys(keys []routeKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Path != keys[j].Path {
			return keys[i].Path < keys[j].Path
		}
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		return keys[i].Prefix < keys[j].Prefix
	})
}

// snapshotRoutes 取 Routes() 的排序后三元组快照。
func snapshotRoutes(svr *Router) []routeKey {
	infos := svr.Routes()
	keys := make([]routeKey, 0, len(infos))
	for _, info := range infos {
		keys = append(keys, routeKey{Method: info.Method, Path: info.Path, Prefix: info.Prefix})
	}
	sortRouteKeys(keys)
	return keys
}

// assertRouteSnapshot 断言 Routes() 快照（排序后）与预期一致。
func assertRouteSnapshot(t *testing.T, svr *Router, want []routeKey) {
	t.Helper()
	got := snapshotRoutes(svr)
	want = append([]routeKey(nil), want...)
	sortRouteKeys(want)
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("route snapshot mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

// recoverPanic 执行 fn 并返回其 panic 内容；fn 未 panic 则判定用例失败。
func recoverPanic(t *testing.T, fn func()) (msg string) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected panic, got none")
		}
		msg = fmt.Sprint(recovered)
	}()
	fn()
	return ""
}

// requireContains 断言 panic 文案里含有全部关键字。
func requireContains(t *testing.T, got string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(got, sub) {
			t.Fatalf("panic message %q does not contain %q", got, sub)
		}
	}
}

// doRequest 通过 GinEngine() 发一次离线请求。
func doRequest(svr *Router, method, path string, hdrs map[string]string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for key, value := range hdrs {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	svr.GinEngine().ServeHTTP(rec, req)
	return rec
}

// ginRouteSet 返回 gin 引擎实际注册的 "METHOD PATH" 集合，用于验证 Routes().Path 与 fullPath 一致。
func ginRouteSet(svr *Router) map[string]bool {
	set := make(map[string]bool)
	for _, route := range svr.GinEngine().Routes() {
		set[route.Method+" "+route.Path] = true
	}
	return set
}
