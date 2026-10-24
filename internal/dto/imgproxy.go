package dto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"go_kanban_service/internal/config"
)

// imgproxyURL кодирует источник в base64: в «plain/s3://…» двойной слэш склеивает
// merge_slashes nginx-шлюза, и подпись перестала бы совпадать с путём, который
// получит imgproxy.
func imgproxyURL(cfg *config.Config, options, source string) string {
	path := "/" + options + "/" + base64.RawURLEncoding.EncodeToString([]byte(source))
	signature := "unsafe"
	if len(cfg.ImgproxyKey) > 0 && len(cfg.ImgproxySalt) > 0 {
		mac := hmac.New(sha256.New, cfg.ImgproxyKey)
		mac.Write(cfg.ImgproxySalt)
		mac.Write([]byte(path))
		signature = base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	return strings.TrimRight(cfg.ImgproxyBaseUrl, "/") + "/" + signature + path
}
