package middleware

import (
	"context"
	"crypto/rsa"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserContextKey contextKey = "user"

// UserClaims описывает структуру данных пользователя из JWT-токена Symfony
type UserClaims struct {
	ID       int64    `json:"id"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
}

type AuthMiddleware struct {
	publicKey *rsa.PublicKey
}

// NewAuthMiddleware загружает публичный ключ RS256 и инициализирует middleware
func NewAuthMiddleware(publicKeyPath string) (*AuthMiddleware, error) {
	keyBytes, err := os.ReadFile(publicKeyPath)
	if err != nil {
		slog.Error("Ошибка чтения публичного ключа JWT", "path", publicKeyPath, "error", err)
		return nil, err
	}

	pubKey, err := jwt.ParseRSAPublicKeyFromPEM(keyBytes)
	if err != nil {
		slog.Error("Ошибка парсинга публичного ключа JWT", "error", err)
		return nil, err
	}

	return &AuthMiddleware{publicKey: pubKey}, nil
}

// Handler проверяет JWT-токен и кладет Claims в контекст запроса
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","code":"unauthorized"}`))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","code":"unauthorized"}`))
			return
		}

		tokenStr := parts[1]

		// Парсим и валидируем токен.
		// Критично: WithExpirationRequired() требует наличие и валидность exp claim.
		// Без этого просроченные/украденные токены будут работать вечно.
		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			// Проверяем метод подписи (должен быть RS256)
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return m.publicKey, nil
		}, jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))

		if err != nil || !token.Valid {
			// Явно 401. Самое важное — WithExpirationRequired() в Parse гарантирует проверку exp.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","code":"unauthorized"}`))
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","code":"unauthorized"}`))
			return
		}

		// Вытаскиваем id, username и roles
		var userClaims UserClaims

		if idVal, exists := claims["id"]; exists {
			switch v := idVal.(type) {
			case float64:
				userClaims.ID = int64(v)
			case int64:
				userClaims.ID = v
			}
		}

		if usernameVal, exists := claims["username"]; exists {
			if usernameStr, ok := usernameVal.(string); ok {
				userClaims.Username = usernameStr
			}
		}

		if rolesVal, exists := claims["roles"]; exists {
			if rolesSlice, ok := rolesVal.([]interface{}); ok {
				for _, rVal := range rolesSlice {
					if rStr, ok := rVal.(string); ok {
						userClaims.Roles = append(userClaims.Roles, rStr)
					}
				}
			}
		}

		// Если ID не найден, токен не подходит для микросервиса
		if userClaims.ID == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","code":"unauthorized"}`))
			return
		}

		// Кладем данные пользователя в контекст запроса
		ctx := context.WithValue(r.Context(), UserContextKey, userClaims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUser достает информацию о пользователе из контекста запроса
func GetUser(ctx context.Context) (UserClaims, bool) {
	user, ok := ctx.Value(UserContextKey).(UserClaims)
	return user, ok
}
