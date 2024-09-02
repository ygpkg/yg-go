package ipgeo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

// GetLocationByIP 函数通过IP地址获取地理位置信息，并返回拼接的字符串
func GetLocationByIP(ip string) (string, error) {
	apiURL := "https://apis.map.qq.com/ws/location/v1/ip"
	// 获取 IP API 的密钥
	ipKey, err := settings.GetText("cook", "IPKey")
	if err != nil {
		logs.Errorf("[cook] [GetLocationByIP] Failed to get ipKey: %s", err.Error())
		return "", fmt.Errorf("failed to get ipKey: %w", err)
	}
	url := fmt.Sprintf("%s?key=%s&ip=%s", apiURL, ipKey, ip)
	resp, err := http.Get(url)
	if err != nil {
		logs.Errorf("[cook] [GetLocationByIP] Failed to get location by IP: %s", err.Error())
		return "", fmt.Errorf("failed to get location by IP: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logs.Errorf("[cook] [GetLocationByIP] Failed to read response body: %s", err.Error())
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		logs.Errorf("[cook] [GetLocationByIP] Failed to unmarshal response body: %s", err.Error())
		return "", fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	// 提取 Nation,Province, City, District 并拼接成字符串
	adInfo := result["result"].(map[string]interface{})["ad_info"].(map[string]interface{})
	nation := adInfo["nation"].(string)
	province := adInfo["province"].(string)
	city := adInfo["city"].(string)
	district := adInfo["district"].(string)
	locationString := fmt.Sprintf("%s%s%s%s", nation, province, city, district)
	return locationString, nil
}
