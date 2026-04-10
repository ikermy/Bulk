package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ikermy/Bulk/internal/metrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type BFFBillingClient struct {
	client *http.Client
	url    string
	token  string
}

// BFFBillingClient — production-реализация клиента для Billing через BFF.
// Используется для проверки баланса (quote) и блокировки транзакций (block)
//
// Соответствие ТЗ §7.3:
// - При отправке запроса Quote клиент добавляет поле `context` с {"source":"bulk"}.
// - Поддерживаются варианты ответа, содержащие либо `canProcess`, либо `canGenerate`.
// - Поле `bySource` может быть как структурой (subscription/credits/wallet), так и
//   плоской картой типа {"subscription": 10, "credits_type1": 5}; Unmarshal обрабатывает оба.
// - `shortfall` поддерживается как объект {"units":..,"amountRequired":..} и как число (units).

func NewBFFBillingClient(url string, timeout time.Duration, token string) *BFFBillingClient {
	// Note: BFF protects /internal/* with a service token (see BFF_TZ p.16.1).
	// The original Bulk_Service_TZ didn't explicitly mandate Bulk to send this token,
	// so we accept an optional token to be compatible with BFF deployments.
	return &BFFBillingClient{client: &http.Client{Timeout: timeout}, url: url, token: token}
}

type QuoteRequest struct {
	UserID string `json:"userId"`
	Count  int    `json:"count"`
	// Context is included to comply with TZ §7.3: e.g. {"source":"bulk"}
	Context map[string]string `json:"context,omitempty"`
}

type SourceBreakdown struct {
	Units     int     `json:"units,omitempty"`
	Remaining int     `json:"remaining,omitempty"`
	Amount    float64 `json:"amount,omitempty"`
}

type QuoteBreakdown struct {
	Subscription SourceBreakdown `json:"subscription,omitempty"`
	Credits      SourceBreakdown `json:"credits,omitempty"`
	Wallet       SourceBreakdown `json:"wallet,omitempty"`
}

type Shortfall struct {
	Units          int     `json:"units"`
	AmountRequired float64 `json:"amountRequired"`
}
type QuoteResponse struct {
	// canProcess is the canonical field; some BFFs may return canGenerate instead
	CanProcess   bool           `json:"canProcess"`
	CanGenerate  bool           `json:"canGenerate,omitempty"`
	Partial      bool           `json:"partial"`
	Requested    int            `json:"requested"`
	AllowedTotal int            `json:"allowedTotal"`
	UnitPrice    float64        `json:"unitPrice,omitempty"`
	BySource     QuoteBreakdown `json:"bySource"`
	Shortfall    *Shortfall     `json:"shortfall,omitempty"`
}

// UnmarshalJSON implements tolerant unmarshalling to support multiple BFF variants
// of the QuoteResponse shape (canGenerate vs canProcess, bySource as object or map,
// and shortfall as object or number).
func (qr *QuoteResponse) UnmarshalJSON(data []byte) error {
	type rawResp struct {
		CanProcess   bool            `json:"canProcess"`
		CanGenerate  bool            `json:"canGenerate"`
		Partial      bool            `json:"partial"`
		Requested    int             `json:"requested"`
		AllowedTotal int             `json:"allowedTotal"`
		UnitPrice    float64         `json:"unitPrice"`
		BySource     json.RawMessage `json:"bySource"`
		Shortfall    json.RawMessage `json:"shortfall"`
	}
	var r rawResp
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	qr.CanProcess = r.CanProcess || r.CanGenerate
	qr.CanGenerate = r.CanGenerate
	qr.Partial = r.Partial
	qr.Requested = r.Requested
	qr.AllowedTotal = r.AllowedTotal
	qr.UnitPrice = r.UnitPrice

	// bySource: try structured breakdown first, then flat map
	if len(r.BySource) > 0 {
		var qb QuoteBreakdown
		if err := json.Unmarshal(r.BySource, &qb); err == nil {
			qr.BySource = qb
		} else {
			var m map[string]float64
			if err := json.Unmarshal(r.BySource, &m); err == nil {
				var qb2 QuoteBreakdown
				if v, ok := m["subscription"]; ok {
					qb2.Subscription.Units = int(v)
				}
				if v, ok := m["wallet"]; ok {
					qb2.Wallet.Units = int(v)
				}
				// credits keys may be named "credits" or "credits_type1"
				if v, ok := m["credits"]; ok {
					qb2.Credits.Units = int(v)
				}
				if v, ok := m["credits_type1"]; ok {
					qb2.Credits.Units = int(v)
				}
				qr.BySource = qb2
			}
		}
	}

	// shortfall: object or number
	if len(r.Shortfall) > 0 {
		// find first non-space char
		first := byte(0)
		for _, b := range r.Shortfall {
			if b == ' ' || b == '\n' || b == '\t' || b == '\r' {
				continue
			}
			first = b
			break
		}
		if first == '{' || first == '[' {
			var s Shortfall
			if err := json.Unmarshal(r.Shortfall, &s); err == nil {
				qr.Shortfall = &s
			}
		} else {
			var num float64
			if err := json.Unmarshal(r.Shortfall, &num); err == nil {
				qr.Shortfall = &Shortfall{Units: int(num)}
			}
		}
	}

	return nil
}

func (c *BFFBillingClient) Quote(ctx context.Context, userID string, count int) (*QuoteResponse, error) {
	tracer := otel.Tracer("bulk-service/billing")
	ctx, span := tracer.Start(ctx, "billing.Quote", trace.WithAttributes(attribute.String("billing.method", "Quote"), attribute.String("billing.user_id", userID)))
	defer span.End()
	start := time.Now()
	reqObj := QuoteRequest{UserID: userID, Count: count, Context: map[string]string{"source": "bulk"}}
	b, _ := json.Marshal(reqObj)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url+"/internal/billing/quote", bytes.NewReader(b))
	if err != nil {
		metrics.BillingCallsTotal.WithLabelValues("Quote", "error").Inc()
		metrics.BillingCallDuration.WithLabelValues("Quote").Observe(time.Since(start).Seconds())
		metrics.BFFRequestErrorsTotal.WithLabelValues("Quote").Inc()
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpReq.Header))

	resp, err := c.client.Do(httpReq)
	if err != nil {
		metrics.BillingCallsTotal.WithLabelValues("Quote", "error").Inc()
		metrics.BillingCallDuration.WithLabelValues("Quote").Observe(time.Since(start).Seconds())
		metrics.BFFRequestErrorsTotal.WithLabelValues("Quote").Inc()
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		metrics.BillingCallsTotal.WithLabelValues("Quote", "error").Inc()
		metrics.BillingCallDuration.WithLabelValues("Quote").Observe(time.Since(start).Seconds())
		metrics.BFFRequestErrorsTotal.WithLabelValues("Quote").Inc()
		return nil, fmt.Errorf("billing quote returned status %d: %s", resp.StatusCode, string(body))
	}

	var qr QuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		metrics.BillingCallsTotal.WithLabelValues("Quote", "error").Inc()
		metrics.BillingCallDuration.WithLabelValues("Quote").Observe(time.Since(start).Seconds())
		metrics.BFFRequestErrorsTotal.WithLabelValues("Quote").Inc()
		return nil, err
	}
	metrics.BillingCallsTotal.WithLabelValues("Quote", "success").Inc()
	metrics.BillingCallDuration.WithLabelValues("Quote").Observe(time.Since(start).Seconds())
	return &qr, nil
}

type BlockBatchResponse struct {
	TransactionIDs []string `json:"transactionIds"`
}

func (c *BFFBillingClient) BlockBatch(ctx context.Context, userID string, count int, batchID string) (*BlockBatchResponse, error) {
	tracer := otel.Tracer("bulk-service/billing")
	ctx, span := tracer.Start(ctx, "billing.BlockBatch", trace.WithAttributes(attribute.String("billing.method", "BlockBatch"), attribute.String("billing.user_id", userID), attribute.String("billing.batch_id", batchID)))
	defer span.End()
	start := time.Now()
	reqObj := map[string]any{"userId": userID, "count": count, "batchId": batchID}
	b, _ := json.Marshal(reqObj)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url+"/internal/billing/block-batch", bytes.NewReader(b))
	if err != nil {
		metrics.BFFRequestErrorsTotal.WithLabelValues("BlockBatch").Inc()
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpReq.Header))

	resp, err := c.client.Do(httpReq)
	if err != nil {
		metrics.BFFRequestErrorsTotal.WithLabelValues("BlockBatch").Inc()
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		metrics.BFFRequestErrorsTotal.WithLabelValues("BlockBatch").Inc()
		return nil, fmt.Errorf("billing block-batch returned status %d: %s", resp.StatusCode, string(body))
	}

	var br BlockBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		metrics.BFFRequestErrorsTotal.WithLabelValues("BlockBatch").Inc()
		return nil, err
	}
	metrics.BillingCallsTotal.WithLabelValues("BlockBatch", "success").Inc()
	metrics.BillingCallDuration.WithLabelValues("BlockBatch").Observe(time.Since(start).Seconds())
	return &br, nil
}

// RefundRequest describes payload to refund/unblock transactions
type RefundRequest struct {
	UserID         string   `json:"userId,omitempty"`
	TransactionIDs []string `json:"transactionIds"`
	BatchID        string   `json:"batchId,omitempty"`
}

func (c *BFFBillingClient) RefundTransactions(ctx context.Context, userID string, transactionIDs []string, batchID string) error {
	tracer := otel.Tracer("bulk-service/billing")
	ctx, span := tracer.Start(ctx, "billing.RefundTransactions", trace.WithAttributes(attribute.String("billing.method", "RefundTransactions"), attribute.String("billing.user_id", userID), attribute.String("billing.batch_id", batchID)))
	defer span.End()
	start := time.Now()
	reqObj := RefundRequest{UserID: userID, TransactionIDs: transactionIDs, BatchID: batchID}
	b, _ := json.Marshal(reqObj)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url+"/internal/billing/refund", bytes.NewReader(b))
	if err != nil {
		metrics.BillingCallsTotal.WithLabelValues("RefundTransactions", "error").Inc()
		metrics.BillingCallDuration.WithLabelValues("RefundTransactions").Observe(time.Since(start).Seconds())
		metrics.BFFRequestErrorsTotal.WithLabelValues("RefundTransactions").Inc()
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpReq.Header))

	resp, err := c.client.Do(httpReq)
	if err != nil {
		metrics.BillingCallsTotal.WithLabelValues("RefundTransactions", "error").Inc()
		metrics.BillingCallDuration.WithLabelValues("RefundTransactions").Observe(time.Since(start).Seconds())
		metrics.BFFRequestErrorsTotal.WithLabelValues("RefundTransactions").Inc()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		metrics.BillingCallsTotal.WithLabelValues("RefundTransactions", "error").Inc()
		metrics.BillingCallDuration.WithLabelValues("RefundTransactions").Observe(time.Since(start).Seconds())
		metrics.BFFRequestErrorsTotal.WithLabelValues("RefundTransactions").Inc()
		return fmt.Errorf("billing refund returned status %d: %s", resp.StatusCode, string(body))
	}
	metrics.BillingCallsTotal.WithLabelValues("RefundTransactions", "success").Inc()
	metrics.BillingCallDuration.WithLabelValues("RefundTransactions").Observe(time.Since(start).Seconds())
	return nil
}

