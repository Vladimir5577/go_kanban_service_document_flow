package dto

import (
	"fmt"
	"strings"

	"go_kanban_service/internal/config"
)

const (
	AvatarSizeThumbnail = "thumbnail"
	AvatarSizeMedium    = "medium"
)

func UserAvatarURL(cfg *config.Config, storageKey *string, size string) *string {
	if cfg == nil || storageKey == nil || strings.TrimSpace(*storageKey) == "" {
		return nil
	}

	width, height := 200, 200
	if size == AvatarSizeThumbnail {
		width, height = 50, 50
	}

	url := fmt.Sprintf(
		"%s/unsafe/rs:fill:%d:%d/plain/s3://%s/%s",
		strings.TrimRight(cfg.ImgproxyBaseUrl, "/"),
		width,
		height,
		cfg.MinioUserBucket,
		*storageKey,
	)

	return &url
}
