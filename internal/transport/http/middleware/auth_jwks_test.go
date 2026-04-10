package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
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

// helper: start a JWKS server and return private key and URL
func startJWKS(t *testing.T) (*rsa.PrivateKey, string, func()) {
	// generate RSA key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// v3: jwk.Import replaces jwk.FromRaw
	key, err := jwk.Import(&priv.PublicKey)
	require.NoError(t, err)
	_ = key.Set(jwk.KeyIDKey, "testkey1")
	// v3: jwa.RS256 is a function — call it to get the value
	_ = key.Set(jwk.AlgorithmKey, jwa.RS256().String())

	set := jwk.NewSet()
	_ = set.AddKey(key)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(set)
	}))

	return priv, srv.URL, func() { srv.Close() }
}

func TestAuthMiddleware_JWKS_AdminAndNonAdmin(t *testing.T) {
	// ensure gin test mode
	gin.SetMode(gin.TestMode)

	priv, jwksURL, closeFn := startJWKS(t)
	defer closeFn()

	// set AUTH_JWKS_URL env
	os.Setenv("AUTH_JWKS_URL", jwksURL)
	defer os.Unsetenv("AUTH_JWKS_URL")

	// build admin token (with role claim)
	tok := jwt.New()
	_ = tok.Set(jwt.IssuerKey, "test-iss")
	_ = tok.Set("role", "admin")
	_ = tok.Set(jwt.IssuedAtKey, time.Now().Unix())
	_ = tok.Set(jwt.ExpirationKey, time.Now().Add(1*time.Hour).Unix())
	// v3: jwk.Import replaces jwk.FromRaw
	jwkPriv, err := jwk.Import(priv)
	require.NoError(t, err)
	_ = jwkPriv.Set(jwk.KeyIDKey, "testkey1")
	// v3: jwa.RS256() — call the function
	signedAdmin, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), jwkPriv))
	require.NoError(t, err)

	// build non-admin token
	tok2 := jwt.New()
	_ = tok2.Set(jwt.IssuerKey, "test-iss")
	_ = tok2.Set(jwt.IssuedAtKey, time.Now().Unix())
	_ = tok2.Set(jwt.ExpirationKey, time.Now().Add(1*time.Hour).Unix())
	jwkPriv2, err := jwk.Import(priv)
	require.NoError(t, err)
	_ = jwkPriv2.Set(jwk.KeyIDKey, "testkey1")
	signedUser, err := jwt.Sign(tok2, jwt.WithKey(jwa.RS256(), jwkPriv2))
	require.NoError(t, err)

	// create gin router with middleware
	rw := httptest.NewRecorder()
	_, r := gin.CreateTestContext(rw)
	r.Use(AuthMiddleware())

	// admin endpoint requires admin
	r.GET("/api/v1/admin/x", func(c *gin.Context) { c.String(http.StatusOK, "admin") })
	// general endpoint
	r.GET("/api/v1/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// non-admin access to general endpoint should pass
	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set("Authorization", "Bearer "+string(signedUser))
	r.ServeHTTP(rw, req)
	require.Equal(t, http.StatusOK, rw.Code)

	// admin endpoint with non-admin token -> forbidden
	rw2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)
	req2.Header.Set("Authorization", "Bearer "+string(signedUser))
	r.ServeHTTP(rw2, req2)
	require.Equal(t, http.StatusForbidden, rw2.Code)

	// admin endpoint with admin token -> ok
	rw3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)
	req3.Header.Set("Authorization", "Bearer "+string(signedAdmin))
	r.ServeHTTP(rw3, req3)
	require.Equal(t, http.StatusOK, rw3.Code)
}

func TestAuthMiddleware_JWKS_InvalidSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// start jwks with one key, but sign token with different key
	_, jwksURL, closeFn := startJWKS(t)
	defer closeFn()
	os.Setenv("AUTH_JWKS_URL", jwksURL)
	defer os.Unsetenv("AUTH_JWKS_URL")

	// generate a different key to sign token (so signature invalid)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tok := jwt.New()
	_ = tok.Set(jwt.IssuerKey, "test-iss")
	_ = tok.Set(jwt.IssuedAtKey, time.Now().Unix())
	// v3: sign with raw key using jwa.RS256()
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), other))
	require.NoError(t, err)

	rw := httptest.NewRecorder()
	_, r := gin.CreateTestContext(rw)
	r.Use(AuthMiddleware())
	r.GET("/api/v1/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set("Authorization", "Bearer "+string(signed))
	r.ServeHTTP(rw, req)
	// invalid signature -> unauthorized
	require.Equal(t, http.StatusUnauthorized, rw.Code)
}
