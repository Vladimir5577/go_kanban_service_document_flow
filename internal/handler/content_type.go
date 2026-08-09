package handler

import (
	"io"
	"net/http"
	"strings"
)

// Тип содержимого вложения определяем сами, а не берём из заголовка загрузки.
//
// Клиент присылает Content-Type в multipart-части, и раньше это значение
// сохранялось в БД и возвращалось обратно на выдаче. В связке с превью,
// которое отдаётся с Content-Disposition: inline, это давало исполнение
// произвольного HTML на origin приложения: достаточно загрузить .html с
// Content-Type: text/html и открыть ссылку превью. Токены доступа лежат в
// localStorage, то есть цена такой ошибки — угон сессии.

// detectionWindow — сколько байт нужно http.DetectContentType.
const detectionWindow = 512

// inlineSafeContentTypes — типы, которые разрешено показывать в браузере
// прямо на нашем origin.
//
// Здесь только растровые изображения. SVG сознательно отсутствует: это XML,
// внутри которого может лежать <script>. PDF тоже отсутствует: просмотрщик
// хоть и в песочнице, но inline-выдача не нужна ни одному экрану — фронт
// рендерит в <img> только то, у чего contentType начинается с image/
// (src/components/Attachments/AttachmentFile.tsx), а всё остальное качает.
var inlineSafeContentTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/gif":  {},
	"image/webp": {},
	"image/bmp":  {},
}

// detectContentType читает начало файла и возвращает тип, определённый по
// содержимому. Позиция чтения возвращается в начало, чтобы вызывающий мог
// сразу отдать тот же ридер в хранилище.
//
// Ошибку чтения не считаем фатальной: пустой или очень короткий файл — это
// нормальный случай, для него http.DetectContentType вернёт
// application/octet-stream, что нас устраивает.
func detectContentType(file io.ReadSeeker) (string, error) {
	buf := make([]byte, detectionWindow)

	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	return http.DetectContentType(buf[:n]), nil
}

// normalizeContentType приводит тип к тому, что безопасно отдавать, и
// сообщает, можно ли показывать файл inline.
//
// Применяется и на выдаче тоже, а не только при загрузке: в БД остались
// строки, записанные до этой правки, — там лежит то, что прислал клиент.
func normalizeContentType(stored string) (contentType string, inlineAllowed bool) {
	base := strings.TrimSpace(strings.ToLower(stored))
	if idx := strings.IndexByte(base, ';'); idx >= 0 {
		base = strings.TrimSpace(base[:idx])
	}

	if _, ok := inlineSafeContentTypes[base]; ok {
		return base, true
	}

	return "application/octet-stream", false
}
