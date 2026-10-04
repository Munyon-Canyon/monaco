package openapi

//go:generate go run -C .. ./cmd/monacoctl gen errors ../../packages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift
//go:generate go run -C .. ./cmd/monacoctl gen openapi api/spec api/openapi.yaml
//go:generate go run -C .. ./cmd/monacoctl gen apis api/spec internal/platform/httpx/api
