public enum AssetStatsInfoCopy {
    public static let title = "Market stats"

    public static let lines: [AssetStatCell] = [
        AssetStatCell(
            label: AssetStats.marketCapLabel,
            value: "What every share of the company is worth at today's price."),
        AssetStatCell(label: AssetStats.volumeLabel, value: "How much of the stock traded in the last 24 hours."),
        AssetStatCell(
            label: AssetStats.openLabel,
            value: "The first price of the current session, or of the last one while the market is closed."),
        AssetStatCell(label: AssetStats.yearLowLabel, value: "The lowest price in the past 52 weeks."),
        AssetStatCell(label: AssetStats.yearHighLabel, value: "The highest price in the past 52 weeks."),
        AssetStatCell(
            label: AssetStats.heldByLabel, value: "How many cabals hold the stock. We never say which ones."),
    ]
}
