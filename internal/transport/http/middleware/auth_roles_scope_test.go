package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/stretchr/testify/require"
)

func TestAuthMiddleware_JWKS_AdminViaRolesAndScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// reuse existing startJWKS helper
	priv, jwksURL, closeFn := startJWKS(t)
	defer closeFn()
	os.Setenv("AUTH_JWKS_URL", jwksURL)
	defer os.Unsetenv("AUTH_JWKS_URL")

	// token with roles array containing admin
	tok := jwt.New()
	_ = tok.Set(jwt.IssuedAtKey, time.Now().Unix())
	_ = tok.Set(jwt.ExpirationKey, time.Now().Add(1*time.Hour).Unix())
	_ = tok.Set("roles", []interface{}{"user", "admin"})
	// v3: jwk.Import replaces jwk.FromRaw
	jwkPriv, err := jwk.Import(priv)
	require.NoError(t, err)
	_ = jwkPriv.Set(jwk.KeyIDKey, "testkey1")
	// v3: jwa.RS256() — call the function
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), jwkPriv))
	require.NoError(t, err)

	rw := httptest.NewRecorder()
	_, r := gin.CreateTestContext(rw)
	r.Use(AuthMiddleware())
	r.GET("/api/v1/admin/x", func(c *gin.Context) { c.String(http.StatusOK, "admin") })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)
	req.Header.Set("Authorization", "Bearer "+string(signed))
	r.ServeHTTP(rw, req)
	require.Equal(t, http.StatusOK, rw.Code)

	// token with scope string containing admin
	tok2 := jwt.New()
	_ = tok2.Set(jwt.IssuedAtKey, time.Now().Unix())
	_ = tok2.Set(jwt.ExpirationKey, time.Now().Add(1*time.Hour).Unix())
	_ = tok2.Set("scope", "read write admin")
	jwkPriv2, err := jwk.Import(priv)
	require.NoError(t, err)
	_ = jwkPriv2.Set(jwk.KeyIDKey, "testkey1")
	signed2, err := jwt.Sign(tok2, jwt.WithKey(jwa.RS256(), jwkPriv2))
	require.NoError(t, err)

	rw2 := httptest.NewRecorder()
	_, r2 := gin.CreateTestContext(rw2)
	r2.Use(AuthMiddleware())
	r2.GET("/api/v1/admin/x", func(c *gin.Context) { c.String(http.StatusOK, "admin") })
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)
	req2.Header.Set("Authorization", "Bearer "+string(signed2))
	r2.ServeHTTP(rw2, req2)
	require.Equal(t, http.StatusOK, rw2.Code)
}
