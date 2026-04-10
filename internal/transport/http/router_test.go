package http

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    cfg "github.com/ikermy/Bulk/internal/config"
    "github.com/stretchr/testify/require"
)

func TestReady_DefaultReady(t *testing.T) {
    // enable legacy tokens for AuthMiddleware and set ADMIN_JWT so requests pass
    os.Setenv("ADMIN_JWT", "admintoken")
    defer os.Unsetenv("ADMIN_JWT")
    r := NewRouter(nil, nil)
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/ready", nil)
    req.Header.Set("Authorization", "Bearer admintoken")
    r.ServeHTTP(w, req)
    require.Equal(t, http.StatusOK, w.Code)
    var out map[string]any
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
    require.Equal(t, "ready", out["status"].(string))
}

func TestReady_KafkaConfigured_ProducerMissing(t *testing.T) {
    cfg := &cfg.Config{}
    cfg.Kafka.Brokers = "kafka:9092"
    os.Setenv("ADMIN_JWT", "admintoken")
    defer os.Unsetenv("ADMIN_JWT")
    r := NewRouter(cfg, nil)
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/ready", nil)
    req.Header.Set("Authorization", "Bearer admintoken")
    r.ServeHTTP(w, req)
    require.Equal(t, http.StatusServiceUnavailable, w.Code)
    var out map[string]any
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
    require.Equal(t, "unready", out["status"].(string))
    require.Equal(t, "producer_missing", out["kafka"].(string))
}



