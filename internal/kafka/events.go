package kafka

import "time"

// BulkJobEventMetadata содержит служебные поля сообщения (ТЗ §8.2 metadata block)
type BulkJobEventMetadata struct {
	Source        string `json:"source"`
	CorrelationID string `json:"correlationId"`
}

// BulkJobEvent — сообщение топика bulk.job (ТЗ §8.2 / §7.2).
// Содержит всю информацию, необходимую BFF для генерации одного баркода.
type BulkJobEvent struct {
	EventType            string               `json:"eventType"`
	JobID                string               `json:"jobId"`
	BatchID              string               `json:"batchId"`
	UserID               string               `json:"userId"`
	RowNumber            int                  `json:"rowNumber"`
	Revision             string               `json:"revision"`
	Fields               map[string]string    `json:"fields"`
	// BillingPreApproved указывает, что транзакция уже предодобрена (ТЗ §7.2)
	BillingPreApproved   bool                 `json:"billingPreApproved"`
	// TransactionId — идентификатор pre-approved транзакции биллинга (ТЗ §7.2)
	TransactionId        string               `json:"transactionId,omitempty"`
	// BillingTransactionId — обратная совместимость
	BillingTransactionId string               `json:"billingTransactionId,omitempty"`
	// Metadata содержит source и correlationId для трассировки (ТЗ §8.2)
	Metadata             BulkJobEventMetadata `json:"metadata"`
	Timestamp            time.Time            `json:"timestamp"`
}

type BulkResultEvent struct {
	EventType   string            `json:"eventType"`
	JobID       string            `json:"jobId"`
	BatchID     string            `json:"batchId"`
	Status      string            `json:"status"`
	BuildID     string            `json:"buildId"`
	BarcodeURLs map[string]string `json:"barcodeUrls"`
	// Billing contains billing result information (status and transactionId)
	// Added to comply with TZ §8.3 which specifies billing block in bulk.result messages.
	Billing     *struct {
		Status        string `json:"status,omitempty"`
		TransactionId string `json:"transactionId,omitempty"`
	} `json:"billing,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
}
