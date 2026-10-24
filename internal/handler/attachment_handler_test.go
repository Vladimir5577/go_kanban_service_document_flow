package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/config"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/service"
)

type uploadAccessStub struct {
	service.AttachmentServiceInterface
	err error
}

func (s uploadAccessStub) AuthorizeUpload(context.Context, int64) error { return s.err }

type unreadBody struct{ reads int }

func (b *unreadBody) Read([]byte) (int, error) { b.reads++; return 0, io.EOF }
func (b *unreadBody) Close() error             { return nil }

func attachmentRequest(body io.Reader) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/cards/1/attachments", body)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("cardId", "1")
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, ctx))
}

func TestUploadRejectsForbiddenBeforeReadingBody(t *testing.T) {
	body := &unreadBody{}
	h := NewAttachmentHandler(uploadAccessStub{err: apperr.ErrForbidden}, nil, &config.Config{})
	w := httptest.NewRecorder()
	h.UploadAttachment()(w, attachmentRequest(body))
	if w.Code != http.StatusForbidden || body.reads != 0 {
		t.Fatalf("status=%d reads=%d", w.Code, body.reads)
	}
}

func TestUploadRejectsOversizedBodyBeforeMinio(t *testing.T) {
	// No Content-Length: exercise the reader limit, not just a header check.
	boundary := "test-boundary"
	start := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"file\"; filename=\"large.bin\"\r\n\r\n"
	r := attachmentRequest(io.MultiReader(strings.NewReader(start), io.LimitReader(zeroReader{}, (50<<20)+1), strings.NewReader("\r\n--"+boundary+"--\r\n")))
	r.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	h := NewAttachmentHandler(uploadAccessStub{}, nil, &config.Config{})
	w := httptest.NewRecorder()
	h.UploadAttachment()(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

type previewServiceStub struct {
	service.AttachmentServiceInterface
	err error
}

func (s previewServiceStub) GetAttachment(context.Context, int64, int64, service.Role) (*model.Attachment, error) {
	return &model.Attachment{ID: 1, CardID: 1, Filename: "fake.png", ContentType: "image/png", StorageKey: "key"}, s.err
}

type previewStorageStub struct {
	service.MinioServiceInterface
	calls int
}

func (s *previewStorageStub) GetObject(context.Context, string, string) (io.ReadCloser, error) {
	s.calls++
	return io.NopCloser(strings.NewReader("<html><script>window.marker=1</script></html>")), nil
}

func TestPreviewSniffsLegacyContentAndChecksAccessBeforeStorage(t *testing.T) {
	for _, denied := range []bool{false, true} {
		storage := &previewStorageStub{}
		var err error
		if denied {
			err = apperr.ErrForbidden
		}
		h := NewAttachmentHandler(previewServiceStub{err: err}, storage, &config.Config{})
		r := attachmentRequest(nil)
		chi.RouteContext(r.Context()).URLParams.Add("id", "1")
		w := httptest.NewRecorder()
		h.PreviewAttachment()(w, r)
		if denied {
			if w.Code != 403 || storage.calls != 0 {
				t.Fatalf("status=%d calls=%d", w.Code, storage.calls)
			}
			continue
		}
		if storage.calls != 1 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("headers=%v calls=%d", w.Header(), storage.calls)
		}
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
			t.Fatal("trusted forged image MIME")
		}
	}
}

func TestPreviewOnlyInlinesDetectedRasterImages(t *testing.T) {
	for _, tc := range []struct{ body, disposition, mime string }{
		{"<html><script>window.marker=1</script></html>", "attachment", "text/html; charset=utf-8"},
		{"<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", "attachment", "text/plain; charset=utf-8"},
		{"%PDF-1.7", "attachment", "application/pdf"},
		{"\x89PNG\r\n\x1a\nrest", "inline", "image/png"},
	} {
		w := httptest.NewRecorder()
		if err := writeAttachmentPreview(w, strings.NewReader(tc.body), "test.html"); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(w.Header().Get("Content-Disposition"), tc.disposition) {
			t.Fatalf("disposition=%s", w.Header().Get("Content-Disposition"))
		}
		if w.Header().Get("Content-Type") != tc.mime || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("headers=%v", w.Header())
		}
		if w.Body.String() != tc.body {
			t.Fatal("body changed")
		}
	}
}
