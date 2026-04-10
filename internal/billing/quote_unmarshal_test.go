package billing

import (
    "encoding/json"
    "testing"
)

func TestQuoteResponse_Unmarshal_Full(t *testing.T) {
    js := `{
        "canProcess": true,
        "partial": false,
        "requested": 5,
        "allowedTotal": 5,
        "unitPrice": 0.5,
        "bySource": {
            "subscription": {"units": 3, "remaining": 27},
            "credits": {"units": 2, "remaining": 18},
            "wallet": {"units": 0, "amount": 0}
        }
    }`

    var qr QuoteResponse
    if err := json.Unmarshal([]byte(js), &qr); err != nil {
        t.Fatalf("unmarshal failed: %v", err)
    }

    if !qr.CanProcess {
        t.Fatalf("expected CanProcess=true, got false")
    }
    if qr.Partial {
        t.Fatalf("expected Partial=false, got true")
    }
    if qr.Requested != 5 {
        t.Fatalf("expected Requested=5, got %d", qr.Requested)
    }
    if qr.AllowedTotal != 5 {
        t.Fatalf("expected AllowedTotal=5, got %d", qr.AllowedTotal)
    }
    if qr.UnitPrice != 0.5 {
        t.Fatalf("expected UnitPrice=0.5, got %v", qr.UnitPrice)
    }
    if qr.BySource.Subscription.Units != 3 {
        t.Fatalf("expected subscription.units=3, got %d", qr.BySource.Subscription.Units)
    }
    if qr.BySource.Subscription.Remaining != 27 {
        t.Fatalf("expected subscription.remaining=27, got %d", qr.BySource.Subscription.Remaining)
    }
    if qr.BySource.Credits.Units != 2 {
        t.Fatalf("expected credits.units=2, got %d", qr.BySource.Credits.Units)
    }
}

func TestQuoteResponse_Unmarshal_Partial(t *testing.T) {
    js := `{
        "canProcess": true,
        "partial": true,
        "requested": 120,
        "allowedTotal": 60,
        "unitPrice": 0.5,
        "bySource": {
            "subscription": {"units": 30, "remaining": 0},
            "credits": {"units": 20, "remaining": 0},
            "wallet": {"units": 10, "amount": 5}
        },
        "shortfall": {"units": 60, "amountRequired": 30}
    }`

    var qr QuoteResponse
    if err := json.Unmarshal([]byte(js), &qr); err != nil {
        t.Fatalf("unmarshal failed: %v", err)
    }

    if !qr.Partial {
        t.Fatalf("expected Partial=true, got false")
    }
    if qr.Requested != 120 {
        t.Fatalf("expected Requested=120, got %d", qr.Requested)
    }
    if qr.AllowedTotal != 60 {
        t.Fatalf("expected AllowedTotal=60, got %d", qr.AllowedTotal)
    }
    if qr.BySource.Subscription.Units != 30 {
        t.Fatalf("expected subscription.units=30, got %d", qr.BySource.Subscription.Units)
    }
    if qr.BySource.Credits.Units != 20 {
        t.Fatalf("expected credits.units=20, got %d", qr.BySource.Credits.Units)
    }
    if qr.BySource.Wallet.Units != 10 {
        t.Fatalf("expected wallet.units=10, got %d", qr.BySource.Wallet.Units)
    }
    if qr.Shortfall == nil {
        t.Fatalf("expected shortfall present")
    }
    if qr.Shortfall.Units != 60 {
        t.Fatalf("expected shortfall.units=60, got %d", qr.Shortfall.Units)
    }
    if qr.Shortfall.AmountRequired != 30 {
        t.Fatalf("expected shortfall.amountRequired=30, got %v", qr.Shortfall.AmountRequired)
    }
}

func TestQuoteResponse_Unmarshal_TZVariant(t *testing.T) {
    js := `{
        "canGenerate": true,
        "partial": true,
        "requested": 120,
        "allowedTotal": 60,
        "unitPrice": 0.5,
        "bySource": {
            "subscription": 30,
            "credits_type1": 20,
            "wallet": 10
        },
        "shortfall": 60
    }`

    var qr QuoteResponse
    if err := json.Unmarshal([]byte(js), &qr); err != nil {
        t.Fatalf("unmarshal failed: %v", err)
    }

    // canGenerate should map to CanProcess
    if !qr.CanProcess {
        t.Fatalf("expected CanProcess=true (from canGenerate), got false")
    }
    if !qr.Partial {
        t.Fatalf("expected Partial=true, got false")
    }
    if qr.Requested != 120 {
        t.Fatalf("expected Requested=120, got %d", qr.Requested)
    }
    if qr.AllowedTotal != 60 {
        t.Fatalf("expected AllowedTotal=60, got %d", qr.AllowedTotal)
    }
    if qr.BySource.Subscription.Units != 30 {
        t.Fatalf("expected subscription.units=30, got %d", qr.BySource.Subscription.Units)
    }
    if qr.BySource.Credits.Units != 20 {
        t.Fatalf("expected credits.units=20, got %d", qr.BySource.Credits.Units)
    }
    if qr.BySource.Wallet.Units != 10 {
        t.Fatalf("expected wallet.units=10, got %d", qr.BySource.Wallet.Units)
    }
    if qr.Shortfall == nil {
        t.Fatalf("expected shortfall present")
    }
    if qr.Shortfall.Units != 60 {
        t.Fatalf("expected shortfall.units=60, got %d", qr.Shortfall.Units)
    }
}

