package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRefundTransactions_Non200ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := NewBFFBillingClient(srv.URL, 2*time.Second, "")
	err := c.RefundTransactions(context.Background(), "user1", []string{"t1"}, "batch1")
	require.Error(t, err)
}

func TestRefundTransactions_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewBFFBillingClient(srv.URL, 2*time.Second, "")
	err := c.RefundTransactions(context.Background(), "user1", []string{"t1", "t2"}, "batch1")
	require.NoError(t, err)
}
