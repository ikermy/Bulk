package kafka

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestStubProducer_Publish_ErrorPath(t *testing.T) {
    s := NewStubProducer()
    // pass a value that json.Marshal cannot encode (channel) to trigger error path
    err := s.Publish(context.Background(), "topic1", nil, map[string]interface{}{"bad": make(chan int)})
    require.Error(t, err)
}

