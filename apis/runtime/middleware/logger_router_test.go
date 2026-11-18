package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/apis/runtime/server"
	"github.com/ygpkg/yg-go/logs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const routerBizMarker = "router-handler-biz-log"

func routerBufferLogger(buf *bytes.Buffer) gin.HandlerFunc {
	cfg := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(cfg),
		zapcore.AddSync(buf),
		zap.InfoLevel,
	)
	sugar := zap.New(core).Sugar()
	return func(ctx *gin.Context) {
		logs.SetContextLogger(ctx, sugar)
		ctx.Next()
	}
}

// newRouterEngine 组装裸 NewRouter，只在引擎链最前面插一条捕获中间件。
// router() 里 IdentityFields 是框架中间件最后一环，WithMiddleware 追加的中间件排在它之后，
// 在那里换 logger 会把已经写入的 companyid/uin 一起换掉；捕获点必须在身份字段写入之前。
// 路由直接注册在引擎上，才能拿到插入后的完整链路。
func newRouterEngine(t *testing.T, buf *bytes.Buffer, opts ...server.RouterOption) (*server.Router, *gin.Engine) {
	t.Helper()
	svr := server.NewRouter("/v1", opts...)
	eng := svr.GinEngine()
	eng.Handlers = append(gin.HandlersChain{routerBufferLogger(buf)}, eng.Handlers...)
	return svr, eng
}

func routerOutputLines(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func routerLineWith(t *testing.T, lines []string, substr string) string {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, substr) {
			return line
		}
	}
	t.Fatalf("no log line contains %q, got: %v", substr, lines)
	return ""
}

func routerAssertKeyOnce(t *testing.T, line, key string) {
	t.Helper()
	if n := strings.Count(line, `"`+key+`":`); n != 1 {
		t.Fatalf("key %q appears %d times in log line: %s", key, n, line)
	}
}

// 验收 1 + 2：裸 NewRouter + 一条路由，访问日志里 companyid / uin 各出现一次、匿名值为 0，
// handler 内的业务日志同样带这两个键。
func TestNewRouterLogsIdentityFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("anonymous writes zero", func(t *testing.T) {
		buf := &bytes.Buffer{}
		_, eng := newRouterEngine(t, buf)
		eng.POST("/v1/video.Report", func(ctx *gin.Context) {
			logs.InfoContextf(ctx, routerBizMarker)
			ctx.String(http.StatusOK, "ok")
		})

		eng.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/video.Report", nil))

		lines := routerOutputLines(t, buf)
		access := routerLineWith(t, lines, `"uri":`)
		routerAssertKeyOnce(t, access, "companyid")
		routerAssertKeyOnce(t, access, "uin")
		if !strings.Contains(access, `"companyid":0`) || !strings.Contains(access, `"uin":0`) {
			t.Fatalf("expected companyid=0 uin=0 in access log, got: %s", access)
		}

		biz := routerLineWith(t, lines, routerBizMarker)
		if !strings.Contains(biz, `"companyid":0`) || !strings.Contains(biz, `"uin":0`) {
			t.Fatalf("expected companyid=0 uin=0 in business log, got: %s", biz)
		}
		routerAssertKeyOnce(t, biz, "companyid")
		routerAssertKeyOnce(t, biz, "uin")
	})

	// 验收 6：注入器必须跑在 IdentityFields 之前（即 svr.Inject 的位置），
	// 否则 IdentityFields 读到的 idmap 还是空的，companyid 恒为 0。
	t.Run("injected idmap reaches both logs", func(t *testing.T) {
		buf := &bytes.Buffer{}
		svr, eng := newRouterEngine(t, buf)
		// 与 app 注册的 auth injector 等价：在 svr.Inject 里填 LoginStatus 的 idmap。
		svr.Default(func(ctx *gin.Context, _ *auth.LoginStatus) error {
			runtime.LoginStatus(ctx).SetID(constants.CtxKeyCompanyID, 42)
			runtime.LoginStatus(ctx).SetID(constants.CtxKeyUin, 7)
			return nil
		})
		eng.POST("/v1/video.Report", func(ctx *gin.Context) {
			logs.InfoContextf(ctx, routerBizMarker)
			ctx.String(http.StatusOK, "ok")
		})

		request := httptest.NewRequest(http.MethodPost, "/v1/video.Report", nil)
		// API Key 分支让 LoginStatus 直接进入 StateSucc，Inject 才会执行注入器（不依赖 JWT/settings）。
		request.Header.Set("Authorization", auth.AuthBearer+auth.AuthAPIKeyPrefix+"test")
		eng.ServeHTTP(httptest.NewRecorder(), request)

		lines := routerOutputLines(t, buf)
		access := routerLineWith(t, lines, `"uri":`)
		routerAssertKeyOnce(t, access, "companyid")
		routerAssertKeyOnce(t, access, "uin")
		if !strings.Contains(access, `"companyid":42`) {
			t.Fatalf("expected companyid=42 in access log, got: %s", access)
		}
		if !strings.Contains(access, `"uin":7`) {
			t.Fatalf("expected uin=7 in access log, got: %s", access)
		}

		biz := routerLineWith(t, lines, routerBizMarker)
		if !strings.Contains(biz, `"companyid":42`) || !strings.Contains(biz, `"uin":7`) {
			t.Fatalf("expected companyid=42 uin=7 in business log, got: %s", biz)
		}
	})
}
