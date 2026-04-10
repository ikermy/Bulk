package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	pkgval "github.com/ikermy/Bulk/pkg/validation"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type BFFValidator struct {
	client *http.Client
	url    string
	// token — служебный токен для внутренних вызовов BFF (/internal/*).
	// Примечание: в Bulk_Service_TZ этот сервисный токен явно не упоминался,
	// однако BFF защищает internal маршруты service token (п.16.1 ТЗ BFF). Поэтому
	// здесь добавлено поле token, которое может быть установлено через конфиг.
	token string
}

// BFFValidator — клиент для Validation BFF: проверка корректности полей/строк (Check fields)

func NewBFFValidator(url string, timeout time.Duration, token string) *BFFValidator {
	return &BFFValidator{client: &http.Client{Timeout: timeout}, url: url, token: token}
}

type ValidateRequest struct {
	Revision string            `json:"revision"`
	Fields   map[string]string `json:"fields"`
}

// ValidateRow calls Validation BFF and returns detailed result including per-field errors
func (v *BFFValidator) ValidateRow(ctx context.Context, row map[string]string, revision string) (*pkgval.ValidationResult, error) {
	tracer := otel.Tracer("bulk-service/validator")
	ctx, span := tracer.Start(ctx, "validator.ValidateRow", trace.WithAttributes(attribute.String("validator.revision", revision), attribute.Int("validator.fields_count", len(row))))
	defer span.End()
	start := time.Now()
	reqObj := ValidateRequest{Revision: revision, Fields: row}
	b, _ := json.Marshal(reqObj)

	// Build HTTP request with context so tracing headers can be injected
	httpReq, err := http.NewRequestWithContext(ctx, "POST", v.url+"/internal/validate", bytes.NewReader(b))
	if err != nil {
		span.SetAttributes(attribute.String("validation.result", "error"))
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Add service token if configured. Note: Bulk_Service_TZ didn't explicitly
	// require sending a service token, but BFF protects /internal/* with a service
	// token (see BFF TЗ p.16.1). We include it here to be compatible with BFF.
	if v.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+v.token)
	}
	// Inject OpenTelemetry propagation headers
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(httpReq.Header))

	resp, err := v.client.Do(httpReq)
	if err != nil {
		span.SetAttributes(attribute.String("validation.result", "error"))
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		span.SetAttributes(attribute.String("validation.result", "bff_error"))
		return nil, fmt.Errorf("bff validate returned status %d: %s", resp.StatusCode, string(body))
	}

	var vr pkgval.ValidationResult
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		span.SetAttributes(attribute.String("validation.result", "decode_error"))
		return nil, err
	}
	if vr.Valid {
		span.SetAttributes(attribute.String("validation.result", "valid"))
	} else {
		span.SetAttributes(attribute.String("validation.result", "invalid"))
	}
	_ = time.Since(start)
	return &vr, nil
}
