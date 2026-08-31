package api

//go:generate go tool oapi-codegen -config ../../api/oapi-codegen.yaml ../../api/openapi.yaml
//go:generate go run ./cmd/runtimegen -spec ../../api/openapi.yaml -output ../../web/src/api/validators.generated.ts
