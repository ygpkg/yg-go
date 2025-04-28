package esquery

// BuildMap 用于构造map[string]interface{}的辅助方法
// pairs 为连续的 key-value 对，key 必须是 string 类型，如 ["key", "value", "key2", "value2", ...]
func BuildMap(pairs ...interface{}) Map {
	m := make(map[string]interface{})
	for i := 0; i < len(pairs); i += 2 {
		if i+1 >= len(pairs) {
			// 如果没有成对，跳过
			break
		}
		key, ok := pairs[i].(string)
		if !ok {
			// 如果 key 不是 string 类型，则跳过这一对
			continue
		}
		// 直接把合法的 key 和 value 加入 map
		m[key] = pairs[i+1]
	}
	return m
}
