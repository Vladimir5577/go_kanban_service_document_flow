package service

import (
	"context"
	"io"
)

// Callers need a readable object, not the storage client's concrete type.
type MinioServiceInterface interface {
	UploadFile(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, contentType string) error
	GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, bucketName, objectName string) error
}
