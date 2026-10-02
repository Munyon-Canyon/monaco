package openapi

//go:generate go run -C .. ./cmd/monacoctl gen errors api/spec/base.yaml ../../packages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift
//go:generate go run -C .. ./cmd/monacoctl gen openapi api/spec api/openapi.yaml
