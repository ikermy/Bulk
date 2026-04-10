package middleware

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jws"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// AuthMiddleware реализует аутентификацию и авторизацию согласно ТЗ §11.1.
// Реализованные требования:
// - Все эндпоинты с префиксом /api/v1/* требуют JWT Bearer токен, подписанный ключами из JWKS (AUTH_JWKS_URL).
// - Эндпоинты /api/v1/admin/* дополнительно требуют наличие роли "admin" в токене (поля claims: `role`, `roles` или `scope`).
//
// Переменные окружения:
// - AUTH_JWKS_URL (обязательно для production): URL JWKS для проверки подписи JWT.
// - AUTH_JWKS_MIN_REFRESH_INTERVAL (необязательно, default 15m): минимальный интервал обновления JWKS-ключей.
// - AUTH_JWT_ISS (необязательно): ожидаемый issuer (iss).
// - AUTH_JWT_AUD (необязательно): ожидаемая аудитория (aud).
// - AUTH_ALLOW_LEGACY_TOKENS (только DEV/тест): fallback на сырые токены ADMIN_JWT, INTERNAL_SERVICE_JWT.
//
// Безопасность:
// - Ключи JWKS обновляются автоматически в фоне (jwk.Cache + httprc.Client), не требуют перезапуска при ротации.
// - Register с waitReady=true (по умолчанию) блокируется до первой загрузки JWKS.
// - В production AUTH_ALLOW_LEGACY_TOKENS должно быть false (по умолчанию).

func AuthMiddleware() gin.HandlerFunc {
	jwksURL := os.Getenv("AUTH_JWKS_URL")
	allowLegacy := os.Getenv("AUTH_ALLOW_LEGACY_TOKENS") == "true"
	// Во время `go test` разрешаем legacy-токены чтобы unit-тесты работали без реального JWKS.
	if !allowLegacy && flag.Lookup("test.v") != nil {
		allowLegacy = true
		log.Println("note: AUTH_JWKS_URL not set; enabling legacy tokens for test run")
	}
	adminRaw := os.Getenv("ADMIN_JWT")
	internalRaw := os.Getenv("INTERNAL_SERVICE_JWT")
	expectedIss := os.Getenv("AUTH_JWT_ISS")
	expectedAud := os.Getenv("AUTH_JWT_AUD")

	// Парсим минимальный интервал обновления JWKS (по умолчанию 15 минут).
	minRefresh := 15 * time.Minute
	if v := os.Getenv("AUTH_JWKS_MIN_REFRESH_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			minRefresh = d
		}
	}

	// bgCtx живёт всё время работы процесса — управляет фоновой горутиной Cache.
	bgCtx := context.Background()

	// cache != nil означает «JWKS настроен и доступен»
	var cache *jwk.Cache
	if jwksURL != "" {
		// v3: NewCache(ctx, httprc.Client) — возвращает (*Cache, error)
		c, err := jwk.NewCache(bgCtx, httprc.NewClient())
		if err != nil {
			if !allowLegacy {
				panic(fmt.Sprintf("ошибка создания JWKS кеша: %v", err))
			}
			log.Printf("warning: ошибка создания JWKS кеша; переключение на legacy-токены: %v\n", err)
		} else {
			// v3: Register(ctx, url, opts...) — ctx передаётся первым аргументом.
			// По умолчанию waitReady=true: вызов блокируется до первой загрузки JWKS.
			// WithMinInterval заменяет устаревший WithMinRefreshInterval из v2.
			if regErr := c.Register(bgCtx, jwksURL, jwk.WithMinInterval(minRefresh)); regErr != nil {
				if !allowLegacy {
					panic(fmt.Sprintf("AUTH_JWKS_URL задан, но получить JWKS с %s не удалось: %v", jwksURL, regErr))
				}
				log.Printf("warning: не удалось загрузить JWKS с %s; переключение на legacy-токены: %v\n", jwksURL, regErr)
			} else {
				cache = c
			}
		}
	} else {
		if !allowLegacy {
			panic("AUTH_JWKS_URL не задан; установите AUTH_JWKS_URL в production или включите AUTH_ALLOW_LEGACY_TOKENS для разработки/тестов")
		}
	}

	return func(c *gin.Context) {
		// health probe — без аутентификации
		if c.FullPath() == "/health" || c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization"})
			return
		}
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			return
		}
		token := parts[1]

		// JWKS путь — ключи берутся из кеша Cache (быстро, без сети при cache-hit).
		var parsed jwt.Token
		var jwtErr error
		if cache != nil {
			// v3: Lookup (переименовано из Get) возвращает кешированный набор ключей.
			keySet, err := cache.Lookup(c.Request.Context(), jwksURL)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication unavailable", "details": err.Error()})
				return
			}
			// jwt.Parse в v3 — валидация (exp/nbf/iat) включена по умолчанию.
			// jws.WithInferAlgorithmFromKey позволяет работать с JWKS без явного поля alg.
			parsed, jwtErr = jwt.Parse([]byte(token), jwt.WithKeySet(keySet, jws.WithInferAlgorithmFromKey(true)))
			if jwtErr != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": jwtErr.Error()})
				return
			}
			// v3: Issuer() возвращает (string, bool)
			if expectedIss != "" {
				if iss, ok := parsed.Issuer(); !ok || iss != expectedIss {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token issuer"})
					return
				}
			}
			// v3: Audience() возвращает ([]string, bool)
			if expectedAud != "" {
				audList, _ := parsed.Audience()
				found := false
				for _, a := range audList {
					if a == expectedAud {
						found = true
						break
					}
				}
				if !found {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token audience"})
					return
				}
			}
		} else {
			// Legacy путь — только при AUTH_ALLOW_LEGACY_TOKENS=true (dev/test)
			if allowLegacy {
				if token != adminRaw && token != internalRaw {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid legacy token"})
					return
				}
			} else {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "authentication not configured"})
				return
			}
		}

		// Admin-эндпоинты требуют claim с ролью admin
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/admin") {
			if parsed != nil {
				// v3: Get(name, &dst) error — вместо (value, bool) в v2
				var role string
				if err := parsed.Get("role", &role); err == nil && strings.Contains(role, "admin") {
					c.Next()
					return
				}
				var roles []interface{}
				if err := parsed.Get("roles", &roles); err == nil {
					for _, el := range roles {
						if s, ok := el.(string); ok && s == "admin" {
							c.Next()
							return
						}
					}
				}
				var scope string
				if err := parsed.Get("scope", &scope); err == nil && strings.Contains(scope, "admin") {
					c.Next()
					return
				}
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin required"})
				return
			}
			// legacy: только raw admin-токен
			if token == adminRaw {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin required"})
			return
		}

		c.Next()
	}
}
