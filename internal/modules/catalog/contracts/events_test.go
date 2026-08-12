package contracts

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestEventNameConstants_HaveExpectedAggregateTypePrefix pins down the
// dot-namespaced shape internal/platform/outbox/postgres's
// postgres_publisher.go relies on -- see
// internal/modules/operations/contracts/events_test.go for the reference
// version of this same test.
func TestEventNameConstants_HaveExpectedAggregateTypePrefix(t *testing.T) {
	tests := []struct {
		name       string
		event      string
		wantPrefix string
	}{
		{"product created", EventProductCreated, "product"},
		{"product discount applied", EventProductDiscountApplied, "product"},
		{"recipe promoted", EventRecipePromoted, "recipe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Split(tt.event, ".")
			if len(parts) != 2 {
				t.Fatalf("event name %q must contain exactly one %q, got %d segment(s): %v",
					tt.event, ".", len(parts), parts)
			}
			if parts[0] != tt.wantPrefix {
				t.Errorf("event name %q aggregate-type prefix = %q, want %q", tt.event, parts[0], tt.wantPrefix)
			}
			if parts[1] == "" {
				t.Errorf("event name %q has an empty event-name suffix after %q", tt.event, ".")
			}
		})
	}
}

// TestEventPayloads_SerializeToStableJSON confirms every event payload DTO
// marshals to exactly its expected set of JSON fields.
func TestEventPayloads_SerializeToStableJSON(t *testing.T) {
	fixedTime := time.Date(2026, 8, 12, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name         string
		payload      any
		wantJSONKeys []string
	}{
		{
			name: "ProductCreatedPayload",
			payload: ProductCreatedPayload{
				ProductID:  "product-1",
				Name:       "X-Burguer",
				BasePrice:  15.0,
				OccurredAt: fixedTime,
			},
			wantJSONKeys: []string{"product_id", "name", "base_price", "occurred_at"},
		},
		{
			name: "ProductDiscountAppliedPayload",
			payload: ProductDiscountAppliedPayload{
				ProductID:       "product-1",
				DiscountPercent: 25.0,
				FinalPrice:      11.25,
				OccurredAt:      fixedTime,
			},
			wantJSONKeys: []string{"product_id", "discount_percent", "final_price", "occurred_at"},
		},
		{
			name: "RecipePromotedPayload",
			payload: RecipePromotedPayload{
				RecipeID:   "recipe-1",
				ProductID:  "product-1",
				OccurredAt: fixedTime,
			},
			wantJSONKeys: []string{"recipe_id", "product_id", "occurred_at"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertNoOmitemptyTag(t, tt.payload)

			raw, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("json.Marshal(%T): %v", tt.payload, err)
			}

			var decoded map[string]json.RawMessage
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("json.Unmarshal(%T output): %v", tt.payload, err)
			}

			if len(decoded) != len(tt.wantJSONKeys) {
				t.Fatalf("%T serialized to %d field(s) %v, want exactly %d field(s) %v",
					tt.payload, len(decoded), sortedKeys(decoded), len(tt.wantJSONKeys), tt.wantJSONKeys)
			}
			for _, key := range tt.wantJSONKeys {
				if _, ok := decoded[key]; !ok {
					t.Errorf("%T JSON is missing expected field %q; got fields %v", tt.payload, key, sortedKeys(decoded))
				}
			}
		})
	}
}

// assertNoOmitemptyTag fails the test if any exported field of payload's
// struct type carries a `json:"...,omitempty"` tag.
func assertNoOmitemptyTag(t *testing.T, payload any) {
	t.Helper()

	typ := reflect.TypeOf(payload)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("json")
		if strings.Contains(tag, "omitempty") {
			t.Errorf("field %s.%s has omitempty json tag %q; event payload fields must never be hidden by omitempty",
				typ.Name(), field.Name, tag)
		}
	}
}

// sortedKeys returns the sorted keys of a decoded JSON object, for
// deterministic failure messages.
func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
