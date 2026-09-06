package media

import (
	"regexp"
	"testing"
)

func TestSignImgproxyPathFallsBackToUnsafeWithoutKeys(t *testing.T) {
	const path = "/rs:fit:400:400/plain/s3://kanban/cards/1/a.png"
	for _, tc := range []struct{ key, salt string }{
		{"", ""},
		{"abcd", ""},
		{"", "abcd"},
		{"not-hex", "zz"},
	} {
		if got := SignImgproxyPath(tc.key, tc.salt, path); got != "/unsafe"+path {
			t.Errorf("key=%q salt=%q: got %q, want /unsafe prefix", tc.key, tc.salt, got)
		}
	}
}

func TestSignImgproxyPathProducesSignature(t *testing.T) {
	// Пример из документации imgproxy (Signing the URL): известные key/salt.
	const (
		key  = "943b421c9eb07c830af81030552c86009268de4e532ba2ee2eab8247c6da0881"
		salt = "520f986b998545b4785e0defbc4f3c1203f22de2374a3d53cb7a7fe9fea309c5"
		path = "/rs:fit:300:300/plain/http://img.example.com/pretty/image.jpg"
	)
	got := SignImgproxyPath(key, salt, path)
	re := regexp.MustCompile(`^/[A-Za-z0-9_-]{43}` + regexp.QuoteMeta(path) + `$`)
	if !re.MatchString(got) {
		t.Fatalf("unexpected signed path: %q", got)
	}
	if got != SignImgproxyPath(key, salt, path) {
		t.Fatal("signature must be deterministic")
	}
	if got == SignImgproxyPath(salt, key, path) {
		t.Fatal("swapping key and salt must change the signature")
	}
}
