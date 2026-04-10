package kafka

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventJSON_MarshalUnmarshal(t *testing.T) {
	ev := BulkJobEvent{
		EventType: "job.created",
		JobID:     "jid",
		BatchID:   "bid",
		UserID:    "u1",
		RowNumber: 1,
		Revision:  "v1",
		Fields:    map[string]string{"a": "b"},
		Timestamp: time.Now(),
	}
	b, err := json.Marshal(ev)
	require.NoError(t, err)
	var ev2 BulkJobEvent
	require.NoError(t, json.Unmarshal(b, &ev2))
	require.Equal(t, ev.EventType, ev2.EventType)
	require.Equal(t, ev.JobID, ev2.JobID)
}
