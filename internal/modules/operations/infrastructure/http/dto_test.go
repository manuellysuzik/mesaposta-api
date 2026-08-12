package http_test

// dto_test.go: Tests for dto.go (newStockItemResponse, newPurchaseOrderResponse, etc.)
// are integrated into handler tests (stock_item_get_test.go, purchase_order_create_test.go, etc.)
// to verify JSON serialization end-to-end, not in isolation.
// This keeps encoding tests close to the HTTP serialization they verify.
