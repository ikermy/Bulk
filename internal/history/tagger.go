package history

import (
	"context"
	"time"

	"github.com/ikermy/Bulk/internal/ports"
)

type Tagger struct {
	producer ports.KafkaProducer
	topic    string
}

func NewTagger(producer ports.KafkaProducer, topic string) *Tagger {
	return &Tagger{producer: producer, topic: topic}
}

// TagEvent публикует произвольное событие с указанным типом.
func (t *Tagger) TagEvent(ctx context.Context, eventType string, payload any) error {
	evt := map[string]any{"eventType": eventType, "payload": payload, "ts": time.Now().UTC().Format(time.RFC3339)}
	return t.producer.Publish(ctx, t.topic, nil, evt)
}

// TagBulkID тегирует баркоды с bulk_id (batchId) согласно ТЗ §1.2 "История: Тегирование баркодов bulk_id".
// Вызывается при получении результата генерации (bulk.result) для связи сгенерированных
// баркодов с соответствующим batch'ем.
func (t *Tagger) TagBulkID(ctx context.Context, bulkID string, jobID string, barcodeURLs map[string]string) error {
	evt := map[string]any{
		"eventType":   "bulk.barcode_tagged",
		"bulk_id":     bulkID,
		"jobId":       jobID,
		"barcodeUrls": barcodeURLs,
		"ts":          time.Now().UTC().Format(time.RFC3339),
	}
	return t.producer.Publish(ctx, t.topic, nil, evt)
}

