// Package model defines Context/State-plane query DTOs.
//
// These types are internal to the query boundary: filters, cursors, pages, and
// read results that the query service and its store adapter exchange. They are
// allowed to evolve with the backend, must stay free of transport/runtime
// orchestration details, and must not be treated as external wire contracts.
package model
