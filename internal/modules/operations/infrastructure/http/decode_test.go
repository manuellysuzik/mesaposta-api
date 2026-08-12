package http_test

// decode_test.go: Tests for decode.go are inlined in each handler's _test.go
// (stock_item_get_test.go, purchase_order_create_test.go, etc.) where they
// validate payload unmarshaling and path parameter parsing in context.
// This keeps tests close to the handlers that call decodeJSON/pathID.
