package application

import "github.com/manuellysuzik/mesaposta-api/internal/modules/catalog/domain"

// centsToCurrency converts a Money's int64-cents representation into a
// float64 whole-currency amount (e.g. 350 cents -> 3.5), matching the scale
// contracts.ProductCreatedPayload.BasePrice and
// contracts.ProductDiscountAppliedPayload.FinalPrice are documented to
// carry -- the same conversion
// internal/modules/operations/domain/receiving.go's own centsToCurrency
// applies, duplicated here since that helper is unexported and lives in a
// different module's domain package this one cannot import (architecture
// rule R3).
func centsToCurrency(m domain.Money) float64 {
	return float64(m.Cents()) / 100.0
}

// hundredthsToPercent converts a DiscountPercent's int64-hundredths
// representation into a float64 whole-percentage amount (e.g. 2500
// hundredths -> 25.0), matching the scale
// contracts.ProductDiscountAppliedPayload.DiscountPercent is documented to
// carry.
func hundredthsToPercent(d domain.DiscountPercent) float64 {
	return float64(d.Hundredths()) / 100.0
}
