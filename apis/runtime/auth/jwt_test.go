package auth

import "testing"

// TestJwtSecretResolver 钉住兜底解析器的注册与调用语义。
//
// 这里只测 resolveJwtSecret（不碰 settings/DB）：GetJwtSecret 的两条分支由集成环境覆盖。
func TestJwtSecretResolver(t *testing.T) {
	t.Cleanup(func() { SetJwtSecretResolver(nil) })

	SetJwtSecretResolver(nil)
	if _, ok := resolveJwtSecret("snapnow", nil); ok {
		t.Fatal("未注册解析器时不应返回 ok=true")
	}

	SetJwtSecretResolver(func(issuer string, cause error) ([]byte, bool) {
		if issuer != "snapnow" {
			return nil, false
		}
		return []byte("from-env"), true
	})

	if got, ok := resolveJwtSecret("snapnow", nil); !ok || string(got) != "from-env" {
		t.Fatalf("已注册解析器时应采用其密钥，got=%q ok=%v", got, ok)
	}
	if _, ok := resolveJwtSecret("other", nil); ok {
		t.Fatal("解析器放弃时应返回 ok=false")
	}
}
