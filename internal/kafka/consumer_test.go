package kafka

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func TestHeaderCarrierReader_GetKeysSet(t *testing.T) {
    c := headerCarrierReader{}
    if c.Get("missing") != "" {
        t.Fatalf("expected empty for missing")
    }
    c.Set("k1", "v1")
    if c.Get("k1") != "v1" {
        t.Fatalf("expected v1")
    }
    ks := c.Keys()
    require.Contains(t, ks, "k1")
}

func TestResultConsumer_Close(t *testing.T) {
    // create a consumer with a reader; NewConsumer uses kafka.NewReader
    rc := NewConsumer("", "topic", "group", nil, nil, "dlq", 1, nil)
    // Close should call reader.Close and not panic
    _ = rc.Close()
}


