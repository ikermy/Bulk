package xls

import (
	"encoding/json"
	"strings"
)

// ParseBarcodeURLs разбирает строку BarcodeURLs в пару URL-ов pdf417/code128.
// Поддерживает три формата хранения (ТЗ §8.3 barcodeUrls):
//   - JSON-объект: {"pdf417": "...", "code128": "..."} или с суффиксом _url
//   - JSON-массив строк: ["pdf417_url", "code128_url"]
//   - CSV: "pdf417_url,code128_url"
func ParseBarcodeURLs(s string) (pdf417, code128 string) {
	if s == "" {
		return
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(s), &obj); err == nil {
		tryStr := func(k string) string {
			if v, ok := obj[k]; ok {
				switch vv := v.(type) {
				case string:
					return vv
				default:
					b, _ := json.Marshal(vv)
					return string(b)
				}
			}
			return ""
		}
		if p := tryStr("pdf417"); p != "" {
			pdf417 = p
		}
		if p := tryStr("pdf417_url"); pdf417 == "" && p != "" {
			pdf417 = p
		}
		if c := tryStr("code128"); c != "" {
			code128 = c
		}
		if c := tryStr("code128_url"); code128 == "" && c != "" {
			code128 = c
		}
		return
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		if len(arr) > 0 {
			pdf417 = arr[0]
		}
		if len(arr) > 1 {
			code128 = arr[1]
		}
		return
	}
	parts := strings.Split(s, ",")
	if len(parts) > 0 {
		pdf417 = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		code128 = strings.TrimSpace(parts[1])
	}
	return
}

