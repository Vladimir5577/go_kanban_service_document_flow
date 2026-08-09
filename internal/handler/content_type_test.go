package handler

import (
	"bytes"
	"strings"
	"testing"
)

// Ловушка, ради которой всё затевалось: html, загруженный с заголовком
// image/png, не должен ни сохраниться как картинка, ни уехать на выдачу
// inline. Иначе он исполняется как страница на нашем origin.
func TestDetectContentTypeIgnoresClaimedType(t *testing.T) {
	html := []byte("<html><body><script>alert(1)</script></body></html>")

	got, err := detectContentType(bytes.NewReader(html))
	if err != nil {
		t.Fatalf("определение типа не должно падать: %v", err)
	}

	if !strings.HasPrefix(got, "text/html") {
		t.Errorf("получили %q, ожидали text/html...", got)
	}

	if _, inline := normalizeContentType(got); inline {
		t.Error("html не должен отдаваться inline")
	}
}

func TestDetectContentTypeRewindsReader(t *testing.T) {
	// Сигнатура PNG; после определения ридер обязан вернуться в начало,
	// иначе в хранилище уедет файл без первых 512 байт.
	payload := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 600)...)
	reader := bytes.NewReader(payload)

	got, err := detectContentType(reader)
	if err != nil {
		t.Fatalf("определение типа не должно падать: %v", err)
	}
	if got != "image/png" {
		t.Errorf("тип: получили %q, ожидали image/png", got)
	}

	rest := make([]byte, len(payload))
	n, _ := reader.Read(rest)
	if n != len(payload) {
		t.Errorf("после определения прочитано %d байт из %d — ридер не отмотан", n, len(payload))
	}
}

func TestDetectContentTypeHandlesShortFile(t *testing.T) {
	got, err := detectContentType(bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("пустой файл — не ошибка: %v", err)
	}
	if got == "" {
		t.Error("тип не должен быть пустым")
	}
}

func TestNormalizeContentType(t *testing.T) {
	cases := []struct {
		stored     string
		wantType   string
		wantInline bool
		why        string
	}{
		{"image/png", "image/png", true, "растровая картинка показывается в интерфейсе"},
		{"IMAGE/JPEG", "image/jpeg", true, "регистр не должен влиять"},
		{"image/webp; charset=binary", "image/webp", true, "параметры отбрасываются"},
		{"text/html", "application/octet-stream", false, "исполняемая разметка только вложением"},
		{"image/svg+xml", "application/octet-stream", false, "svg — это xml со скриптом внутри"},
		{"application/pdf", "application/octet-stream", false, "inline не нужен ни одному экрану"},
		{"", "application/octet-stream", false, "пустое значение из старых строк БД"},
		{"application/x-msdownload", "application/octet-stream", false, "исполняемое"},
	}

	for _, c := range cases {
		gotType, gotInline := normalizeContentType(c.stored)
		if gotType != c.wantType || gotInline != c.wantInline {
			t.Errorf("%q -> (%q, %v), ожидали (%q, %v): %s",
				c.stored, gotType, gotInline, c.wantType, c.wantInline, c.why)
		}
	}
}
