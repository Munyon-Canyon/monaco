package domain

func PopularRank(symbol string) int16 {
	return map[string]int16{
		"AAPLx": 1, "MSFTx": 2, "GOOGLx": 3, "AMZNx": 4, "NVDAx": 5, "METAx": 6, "TSLAx": 7, "NFLXx": 8, "COINx": 9,
		"JPMx": 10, "DISx": 11, "WMTx": 12, "AMDx": 13, "INTCx": 14, "PYPLx": 15, "CRMx": 16, "ORCLx": 17,
	}[symbol]
}
