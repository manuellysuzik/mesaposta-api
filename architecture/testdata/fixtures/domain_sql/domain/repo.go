// Package domain is a fixture domain package that imports database/sql
// directly, violating R1 (domain packages must not reach database/sql).
package domain

import "database/sql"

// Ping accepts a *sql.DB so the import above is not unused.
func Ping(db *sql.DB) error {
	return db.Ping()
}
