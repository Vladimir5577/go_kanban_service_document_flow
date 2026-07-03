package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioServiceInterface interface {
	UploadFile(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, contentType string) error
	GetObject(ctx context.Context, bucketName, objectName string) (*minio.Object, error)
	DeleteObject(ctx context.Context, bucketName, objectName string) error
}

type MinioService struct {
	client *minio.Client
}

func NewMinioService(endpoint, accessKeyID, secretAccessKey string, useSSL bool, bucketName string) (*MinioService, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to init minio client: %w", err)
	}

	// Make bucket if not exists
	err = minioClient.MakeBucket(context.Background(), bucketName, minio.MakeBucketOptions{})
	if err != nil {
		exists, errBucketExists := minioClient.BucketExists(context.Background(), bucketName)
		if errBucketExists == nil && exists {
			slog.Info("MinIO bucket already exists", "bucket", bucketName)
		} else {
			return nil, fmt.Errorf("failed to check/create minio bucket: %w", err)
		}
	} else {
		slog.Info("Successfully created MinIO bucket", "bucket", bucketName)
	}

	return &MinioService{client: minioClient}, nil
}

func (s *MinioService) UploadFile(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, contentType string) error {
	_, err := s.client.PutObject(ctx, bucketName, objectName, reader, objectSize, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (s *MinioService) GetObject(ctx context.Context, bucketName, objectName string) (*minio.Object, error) {
	return s.client.GetObject(ctx, bucketName, objectName, minio.GetObjectOptions{})
}

func (s *MinioService) DeleteObject(ctx context.Context, bucketName, objectName string) error {
	return s.client.RemoveObject(ctx, bucketName, objectName, minio.RemoveObjectOptions{})
}
