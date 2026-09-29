package chain

func OnCurve(key []byte) bool { return onCurve(key) }

func DecodeCompactU16(raw []byte) (int, []byte, bool) { return compactU16(raw) }
