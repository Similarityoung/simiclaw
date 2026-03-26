// Package model defines Runtime-plane execution and observe DTOs.
//
// These types represent event loop and worker command/result shapes exchanged
// between runtime orchestration and the store-backed repository adapter. They
// are internal backend contracts, may evolve without wire compatibility
// guarantees, and must not be promoted into generic store or transport DTOs.
package model
