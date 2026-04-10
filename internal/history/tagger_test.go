package history

import (
	"context"
	"errors"
	"testing"

	"github.com/ikermy/Bulk/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestTagEvent_Publishes(t *testing.T) {
	called := false
	mp := &testutil.MockProducer{}
	mp.PublishFn = func(ctx context.Context, topic string, key []byte, msg any) error {
		called = true
		return nil
	}
	tg := NewTagger(mp, "topic")
	require.NoError(t, tg.TagEvent(context.Background(), "ev", map[string]any{"a": 1}))
	require.True(t, called)
}

func TestTagEvent_ReturnsError(t *testing.T) {
	wantErr := errors.New("publish error")
	mp := &testutil.MockProducer{}
	mp.PublishFn = func(ctx context.Context, topic string, key []byte, msg any) error {
		return wantErr
	}
	tg := NewTagger(mp, "topic")
	err := tg.TagEvent(context.Background(), "ev", nil)
	require.ErrorIs(t, err, wantErr)
}

func TestTagEvent_PayloadFields(t *testing.T) {
	var captured any
	mp := &testutil.MockProducer{}
	mp.PublishFn = func(ctx context.Context, topic string, key []byte, msg any) error {
		captured = msg
		return nil
	}
	tg := NewTagger(mp, "my-topic")
	require.NoError(t, tg.TagEvent(context.Background(), "test.event", map[string]any{"x": 42}))
	m, ok := captured.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "test.event", m["eventType"])
	require.NotEmpty(t, m["ts"])
}

func TestTagBulkID_Publishes(t *testing.T) {
	var capturedTopic string
	var capturedMsg any
	mp := &testutil.MockProducer{}
	mp.PublishFn = func(ctx context.Context, topic string, key []byte, msg any) error {
		capturedTopic = topic
		capturedMsg = msg
		return nil
	}
	tg := NewTagger(mp, "trans-history.log")
	urls := map[string]string{"row1": "https://cdn/bar1.png"}
	require.NoError(t, tg.TagBulkID(context.Background(), "batch-1", "job-1", urls))
	require.Equal(t, "trans-history.log", capturedTopic)

	m, ok := capturedMsg.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "bulk.barcode_tagged", m["eventType"])
	require.Equal(t, "batch-1", m["bulk_id"])
	require.Equal(t, "job-1", m["jobId"])
	require.Equal(t, urls, m["barcodeUrls"])
	require.NotEmpty(t, m["ts"])
}

func TestTagBulkID_ReturnsError(t *testing.T) {
	wantErr := errors.New("publish failed")
	mp := &testutil.MockProducer{}
	mp.PublishFn = func(ctx context.Context, topic string, key []byte, msg any) error {
		return wantErr
	}
	tg := NewTagger(mp, "trans-history.log")
	err := tg.TagBulkID(context.Background(), "b1", "j1", nil)
	require.ErrorIs(t, err, wantErr)
}

