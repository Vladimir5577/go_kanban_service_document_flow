package dto

import (
	"fmt"
	"strings"

	"go_kanban_service/internal/config"
	"go_kanban_service/internal/media"
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

	// Подпись при заданных IMGPROXY_KEY/SALT, иначе /unsafe/ (BE-03 / FE-01).
	path := fmt.Sprintf("/rs:fill:%d:%d/plain/s3://%s/%s", width, height, cfg.MinioUserBucket, *storageKey)
	url := strings.TrimRight(cfg.ImgproxyBaseUrl, "/") + media.SignImgproxyPath(cfg.ImgproxyKey, cfg.ImgproxySalt, path)

	return &url
}
