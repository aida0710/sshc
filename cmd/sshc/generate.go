package main

// Parser dispatch, help text and completion vocabulary share clispec as their
// single source. Flag semantics remain in the handwritten parsers.
//go:generate go run ./internal/cligen -output cli_contract.gen.go
