package logs

import "encoding/json"

// JSON format to json
func JSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
