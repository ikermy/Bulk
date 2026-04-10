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

// reuse startJWKS helper from auth_jwks_test.go

func TestAuthMiddleware_JWKS_IssuerAndAudience(t *testing.T) {
	gin.SetMode(gin.TestMode)
	priv, jwksURL, closeFn := startJWKS(t)
	defer closeFn()
	os.Setenv("AUTH_JWKS_URL", jwksURL)
	defer os.Unsetenv("AUTH_JWKS_URL")

	// create token with iss and aud as string
	tok := jwt.New()
	_ = tok.Set(jwt.IssuerKey, "good-iss")
	_ = tok.Set(jwt.AudienceKey, []string{"good-aud"})
	_ = tok.Set(jwt.IssuedAtKey, time.Now().Unix())
	_ = tok.Set(jwt.ExpirationKey, time.Now().Add(1*time.Hour).Unix())
	// v3: jwk.Import replaces jwk.FromRaw
	jwkPriv, err := jwk.Import(priv)
	require.NoError(t, err)
	_ = jwkPriv.Set(jwk.KeyIDKey, "testkey1")
	// v3: jwa.RS256() — call the function
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), jwkPriv))
	require.NoError(t, err)

	// set expected env for iss and aud
	os.Setenv("AUTH_JWT_ISS", "good-iss")
	os.Setenv("AUTH_JWT_AUD", "good-aud")
	defer func() { os.Unsetenv("AUTH_JWT_ISS"); os.Unsetenv("AUTH_JWT_AUD") }()

	rw := httptest.NewRecorder()
	_, r := gin.CreateTestContext(rw)
	r.Use(AuthMiddleware())
	r.GET("/api/v1/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set("Authorization", "Bearer "+string(signed))
	r.ServeHTTP(rw, req)
	t.Logf("response body: %q", rw.Body.String())
	require.Equal(t, http.StatusOK, rw.Code)
}

func TestAuthMiddleware_JWKS_AudienceArrayAndMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	priv, jwksURL, closeFn := startJWKS(t)
	defer closeFn()
	os.Setenv("AUTH_JWKS_URL", jwksURL)
	defer os.Unsetenv("AUTH_JWKS_URL")

	// token with aud as array
	tok := jwt.New()
	_ = tok.Set(jwt.IssuedAtKey, time.Now().Unix())
	_ = tok.Set(jwt.ExpirationKey, time.Now().Add(1*time.Hour).Unix())
	_ = tok.Set(jwt.AudienceKey, []string{"x", "wanted-aud", "y"})
	jwkPriv, err := jwk.Import(priv)
	require.NoError(t, err)
	_ = jwkPriv.Set(jwk.KeyIDKey, "testkey1")
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), jwkPriv))
	require.NoError(t, err)

	os.Setenv("AUTH_JWT_AUD", "wanted-aud")
	defer os.Unsetenv("AUTH_JWT_AUD")

	rw := httptest.NewRecorder()
	_, r := gin.CreateTestContext(rw)
	r.Use(AuthMiddleware())
	r.GET("/api/v1/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set("Authorization", "Bearer "+string(signed))
	r.ServeHTTP(rw, req)
	t.Logf("response body: %q", rw.Body.String())
	require.Equal(t, http.StatusOK, rw.Code)

	// now test missing audience when expected is set -> unauthorized
	rw2 := httptest.NewRecorder()
	os.Setenv("AUTH_JWT_AUD", "no-such-aud")
	defer os.Unsetenv("AUTH_JWT_AUD")
	_, r2 := gin.CreateTestContext(rw2)
	r2.Use(AuthMiddleware())
	r2.GET("/api/v1/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req2.Header.Set("Authorization", "Bearer "+string(signed))
	r2.ServeHTTP(rw2, req2)
	t.Logf("response body: %q", rw2.Body.String())
	require.Equal(t, http.StatusUnauthorized, rw2.Code)
}
