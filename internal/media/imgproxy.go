// Package media — построение ссылок на медиа-стек (imgproxy).
package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// SignImgproxyPath подписывает путь обработки imgproxy (BE-03 / FE-01).
//
// keyHex/saltHex — IMGPROXY_KEY/IMGPROXY_SALT в hex (те же, что у контейнера
// imgproxy и у Symfony App\Service\Media\ImgproxyUrlSigner). Пустые или
// невалидные — путь остаётся «/unsafe» + path, как раньше: включение подписи
// на сервере без ключей в сервисах ломало бы все превью. Порядок выката:
// сначала ключи в обоих сервисах (imgproxy при ALLOW_UNSAFE_URL=1 принимает и
// подписанные URL), потом IMGPROXY_ALLOW_UNSAFE_URL=0.
//
// path начинается со слэша: "/rs:fit:400:400/plain/s3://bucket/key".
// Подпись — base64url без «=» от HMAC-SHA256(key, salt ‖ path).
func SignImgproxyPath(keyHex, saltHex, path string) string {
	key, errKey := hex.DecodeString(keyHex)
	salt, errSalt := hex.DecodeString(saltHex)
	if keyHex == "" || saltHex == "" || errKey != nil || errSalt != nil {
		return "/unsafe" + path
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(salt)
	mac.Write([]byte(path))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return "/" + signature + path
}
