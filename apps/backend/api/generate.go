package openapi

//go:generate go run -C .. ./cmd/monacoctl gen errors api/spec/error_codes.yaml ../../packages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift
//go:generate go run -C .. ./cmd/monacoctl gen openapi api/spec api/openapi.yaml
