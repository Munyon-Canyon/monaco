package domain

import "github.com/monaco/monaco/apps/backend/internal/platform/money"

func FirstDepositThreshold() money.Micros { return money.MicrosFromUint64(10_000_000) }
