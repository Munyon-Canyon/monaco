public enum MarketMover {
    public static func top(in assets: [MarketAsset]) -> MarketAsset? {
        assets
            .filter { $0.changeBasisPoints != nil }
            .max { abs($0.changeBasisPoints ?? 0) < abs($1.changeBasisPoints ?? 0) }
    }
}
