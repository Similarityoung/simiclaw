// Package workers groups named background runtime responsibilities.
//
// Each worker owns one poll loop, stop path, heartbeat identity, and failure
// strategy instead of sharing a single catch-all file.
package workers
