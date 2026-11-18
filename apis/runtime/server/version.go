package server

// 接口版本前缀常量。
//
// 取值是**归一化形式**（前导斜杠、无尾斜杠），与 WithPrefixes / Scope / RouteInfo.Prefix 使用同一口径：
//
//	svr := server.NewRouter(server.WithPrefixes(server.PrefixAPIV3, server.PrefixAPIV4))
//	v3, v4 := svr.Scope(server.PrefixAPIV3), svr.Scope(server.PrefixAPIV4)
//
//	svr.All().P("/common.ping", ping)
//	for _, route := range svr.Routes() {
//	    if route.Prefix == server.PrefixAPIV3 { /* ... */ }
//	}
//
// 两点约束：
//
//   - 不提供 PrefixAPIDefault：新模型没有「默认前缀」，前缀必须在构造期显式声明（决策 D8）。
//
//   - 这些常量不带尾斜杠，不能直接与相对路径做字符串相加；拼接时补一个斜杠：
//
//     eng.GinEngine().Group(server.PrefixAPIV3 + "/account")
//     // 或 path.Join(server.PrefixAPIV3, "account")（注意 path.Join 会吞掉相对路径的尾斜杠）
//
// 多段前缀（如 "/api/v1"）请由服务侧定义完整常量，不要用 "/api" 与 PrefixAPIV1 拼接。
const (
	// PrefixAPIV1 接口 v1 前缀。
	PrefixAPIV1 = "/v1"
	// PrefixAPIV2 接口 v2 前缀。
	PrefixAPIV2 = "/v2"
	// PrefixAPIV3 接口 v3 前缀。
	PrefixAPIV3 = "/v3"
	// PrefixAPIV4 接口 v4 前缀。
	PrefixAPIV4 = "/v4"
	// PrefixAPIV5 接口 v5 前缀。
	PrefixAPIV5 = "/v5"
)
