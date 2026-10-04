import Foundation

extension Components.Schemas.AssetChart {
    public static let oneDay = Self(
        range: ._1d,
        bucketSeconds: 300,
        points: [
            .init(
                t: Date(timeIntervalSince1970: 1_772_596_200),
                openMicros: 174_000_000,
                highMicros: 176_000_000,
                lowMicros: 173_000_000,
                closeMicros: 175_420_000
            )
        ],
        empty: false,
        attribution: "Data provided by CoinGecko"
    )

    public static let empty = Self(
        range: ._1d,
        bucketSeconds: 300,
        points: [],
        empty: true,
        attribution: "Data provided by CoinGecko"
    )
}
