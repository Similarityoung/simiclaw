// Package readmodel contains store-internal read projections.
//
// These records exist to support SQLite scans and query-friendly projection
// shapes inside the store layer. They are not external API contracts and should
// not be imported outside internal/store.
package readmodel
