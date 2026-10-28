import Foundation

extension Components.Schemas.AssetChart {
    public static let oneDay = Self(
        range: ._1d,
        interval: ._5m,
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
        attribution: "Data provided by CoinGecko",
        nextBefore: nil
    )

    public static let empty = Self(
        range: ._1d,
        interval: ._5m,
        bucketSeconds: 300,
        points: [],
        empty: true,
        attribution: "Data provided by CoinGecko",
        nextBefore: nil
    )
}

extension Components.Schemas.AssetChart {
    public static let candles = candles(range: ._1d, interval: ._5m)

    public static func candles(
        range: RangePayload, interval: IntervalPayload, count: Int = 300, sparse: Bool = false
    ) -> Self {
        let width = seconds(interval)
        let lastBar: Int64 = 1_790_085_300
        var state: UInt64 = 0x9E37_79B9_7F4A_7C15
        func next(_ bound: Int64) -> Int64 {
            state = state &* 6_364_136_223_846_793_005 &+ 1_442_695_040_888_963_407
            return Int64(state >> 33) % bound
        }
        var close: Int64 = 172_800_000
        var points: [Components.Schemas.ChartPoint] = []
        for index in 0..<count {
            let open = close
            close = max(open + next(1_600_000) - 780_000, 1_000_000)
            let high = max(open, close) + next(700_000)
            let low = min(open, close) - next(700_000)
            let isFlat = sparse && index % 3 != 0
            points.append(
                Components.Schemas.ChartPoint(
                    t: Date(timeIntervalSince1970: TimeInterval(lastBar - Int64(count - 1 - index) * width)),
                    openMicros: isFlat ? close : open,
                    highMicros: isFlat ? close : high,
                    lowMicros: isFlat ? close : low,
                    closeMicros: close
                )
            )
        }
        return Self(
            range: range,
            interval: interval,
            bucketSeconds: width,
            points: points,
            empty: false,
            attribution: "Data provided by CoinGecko",
            nextBefore: nil,
            previousCloseMicros: range == ._1d ? 170_400_000 : nil
        )
    }

    private static func seconds(_ interval: IntervalPayload) -> Int64 {
        switch interval {
        case ._1m: 60
        case ._5m: 300
        case ._15m: 900
        case ._30m: 1_800
        case ._1h: 3_600
        case ._4h: 14_400
        case ._1d: 86_400
        case ._1w: 604_800
        }
    }
}
