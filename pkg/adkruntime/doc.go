// Package adkruntime contains the scaffold for the ADK runtime track.
//
// The repository currently uses a parallel dual-stack strategy:
// existing pkg/runtime, pkg/runner, and pkg/store remain untouched,
// while new ADK-oriented components are introduced incrementally here.
// This allows side-by-side validation and safer migration without
// behavior drift in the active runtime path.
package adkruntime
