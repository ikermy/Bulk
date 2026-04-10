package http

import (
    "context"
    "testing"
    "time"

    cfg "github.com/ikermy/Bulk/internal/config"
    di "github.com/ikermy/Bulk/internal/di"
    "github.com/stretchr/testify/require"
)

func TestNewServer_ConfigAppliedAndShutdown(t *testing.T) {
    c := &cfg.Config{}
    c.Server.Host = "127.0.0.1"
    c.Server.Port = 12345
    c.Server.ReadTimeout = 1 * time.Second
    c.Server.WriteTimeout = 2 * time.Second

    s := NewServer(c, &di.Deps{})
    require.NotNil(t, s)
    require.Equal(t, "127.0.0.1:12345", s.srv.Addr)
    require.Equal(t, 1*time.Second, s.srv.ReadTimeout)
    require.Equal(t, 2*time.Second, s.srv.WriteTimeout)

    // Shutdown on non-started server should return nil
    require.NoError(t, s.Shutdown(context.Background(), 50*time.Millisecond))
}

