package main

import "github.com/monaco/monaco/apps/backend/internal/errs"

func genErrors(swift string) error { return writeSwiftCases(swift, errs.All()) }
