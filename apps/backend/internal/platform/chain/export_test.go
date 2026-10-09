package chain

func DecodeCompactU16(raw []byte) (int, []byte, bool) { return compactU16(raw) }
