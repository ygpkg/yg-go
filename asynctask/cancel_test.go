package asynctask_test

// 取消信号是「取消快路径」的跨进程契约（Cancel 的 pg_notify 载荷 ↔ 执行侧监听器），
// 编码/解析/匹配三条规则各自被两个包依赖，因此在根包用外部测试固定下来。

import (
	"testing"

	"github.com/ygpkg/yg-go/asynctask"
)

// TestCancelSignalRoundTrip 编解码往返。
func TestCancelSignalRoundTrip(t *testing.T) {
	src := asynctask.CancelSignal{TaskID: "t-1", Generation: 2, Attempt: 7}
	got, ok := asynctask.DecodeCancelSignal(src.Encode())
	if !ok {
		t.Fatal("合法载荷应可解析")
	}
	if got != src {
		t.Fatalf("往返结果 = %+v, want %+v", got, src)
	}
}

// TestDecodeCancelSignalRejectsBadPayload 非法载荷一律丢弃（监听器记日志即可，正确性靠补查）。
func TestDecodeCancelSignalRejectsBadPayload(t *testing.T) {
	for _, payload := range []string{"", "not-json", "{}", `{"generation":1}`} {
		if _, ok := asynctask.DecodeCancelSignal(payload); ok {
			t.Fatalf("载荷 %q 应被拒绝", payload)
		}
	}
}

// TestCancelSignalMatch 匹配规则：id 必须相等；generation/attempt 为 0 视作通配。
func TestCancelSignalMatch(t *testing.T) {
	sig := asynctask.CancelSignal{TaskID: "t-1", Generation: 2, Attempt: 3}
	if !sig.Match("t-1", 2, 3) {
		t.Fatal("完全一致应匹配")
	}
	if sig.Match("t-1", 3, 3) {
		t.Fatal("generation 不一致不该匹配（晚到取消不得影响新执行）")
	}
	if sig.Match("t-1", 2, 4) {
		t.Fatal("attempt 不一致不该匹配")
	}
	if sig.Match("t-2", 2, 3) {
		t.Fatal("任务 id 不一致不该匹配")
	}

	wild := asynctask.CancelSignal{TaskID: "t-1"}
	if !wild.Match("t-1", 99, 99) {
		t.Fatal("通配信号应匹配同一任务的任意执行")
	}
	if wild.Match("t-2", 99, 99) {
		t.Fatal("通配信号仍受任务 id 限制")
	}
}
