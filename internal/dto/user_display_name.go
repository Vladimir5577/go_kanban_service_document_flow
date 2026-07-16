package dto

import (
	"strings"

	"go_kanban_service/internal/model"
)

func UserDisplayName(user model.User) string {
	parts := []string{
		strings.TrimSpace(user.Lastname),
		strings.TrimSpace(user.Firstname),
	}

	return strings.Join(compactStrings(parts), " ")
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
