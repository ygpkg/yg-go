package server

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// TestVersionPrefixConstants 钉住版本前缀常量的口径：
// 归一化形式（前导斜杠、无尾斜杠），可直接用于 WithPrefixes / Scope / Routes() 比较。
func TestVersionPrefixConstants(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"PrefixAPIV1", PrefixAPIV1, "/v1"},
		{"PrefixAPIV2", PrefixAPIV2, "/v2"},
		{"PrefixAPIV3", PrefixAPIV3, "/v3"},
		{"PrefixAPIV4", PrefixAPIV4, "/v4"},
		{"PrefixAPIV5", PrefixAPIV5, "/v5"},
	}
	seen := make(map[string]string, len(cases))
	for _, testCase := range cases {
		if testCase.got != testCase.want {
			t.Fatalf("%s = %q, want %q", testCase.name, testCase.got, testCase.want)
		}
		if prev, ok := seen[testCase.got]; ok {
			t.Fatalf("%s and %s share the same value %q", prev, testCase.name, testCase.got)
		}
		seen[testCase.got] = testCase.name
	}

	svr := NewRouter(WithPrefixes(PrefixAPIV3, PrefixAPIV4))
	if svr.prefixes[0] != PrefixAPIV3 || svr.prefixes[1] != PrefixAPIV4 {
		t.Fatalf("prefixes = %v, want [%s %s]", svr.prefixes, PrefixAPIV3, PrefixAPIV4)
	}

	// Scope 用常量取作用域，RouteInfo.Prefix 与常量逐字相等（都无尾斜杠）。
	v3 := svr.Scope(PrefixAPIV3)
	v3.P("/user.login", func(*gin.Context) {})
	routes := svr.Routes()
	if len(routes) != 1 || routes[0].Prefix != PrefixAPIV3 || routes[0].Path != PrefixAPIV3+"/user.login" {
		t.Fatalf("Routes() = %+v, want one route under %s", routes, PrefixAPIV3)
	}

	// 传入带尾斜杠的旧写法仍被归一化到常量口径（方便 1.23 线迁移）。
	legacy := NewRouter(WithPrefixes(PrefixAPIV3 + "/"))
	if legacy.prefixes[0] != PrefixAPIV3 {
		t.Fatalf("legacy prefix %q normalized to %q, want %q", PrefixAPIV3+"/", legacy.prefixes[0], PrefixAPIV3)
	}

	// 未声明的版本常量不能用于注册。
	msg := recoverPanic(t, func() { svr.Scope(PrefixAPIV5) })
	requireContains(t, msg, "undeclared prefix", PrefixAPIV5)
}
