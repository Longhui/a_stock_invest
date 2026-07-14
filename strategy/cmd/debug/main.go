package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func main() {
	url := "http://push2.eastmoney.com/api/qt/stock/get?secid=1.600036&fields=f57,f58,f84,f85,f168"

	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://quote.eastmoney.com/")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var raw map[string]interface{}
	json.Unmarshal(body, &raw)
	fmt.Printf("原始响应:\n%s\n", body)
	fmt.Printf("\n解析后:\n")
	if data, ok := raw["data"].(map[string]interface{}); ok {
		fmt.Printf("f84(总股本) = %v\n", data["f84"])
		fmt.Printf("f85(流通股本) = %v\n", data["f85"])
		fmt.Printf("f168 = %v\n", data["f168"])
	}
}