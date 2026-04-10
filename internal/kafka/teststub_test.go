package kafka

import (
	"context"
	"testing"
)

func TestStubProducer_Publish_SuccessAndError(t *testing.T) {
	p := NewStubProducer()
	// publish a simple value should succeed
	if err := p.Publish(context.Background(), "topic1", nil, map[string]string{"a": "b"}); err != nil {
		t.Fatalf("unexpected error on publish: %v", err)
	}
	// publish a value that cannot be marshaled (channel) should return error
	ch := make(chan int)
	if err := p.Publish(context.Background(), "topic1", nil, ch); err == nil {
		t.Fatalf("expected error when publishing non-marshallable value")
	}
}
