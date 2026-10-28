public enum ChartInterval: String, CaseIterable, Codable, Sendable {
    case m1 = "1m"
    case m5 = "5m"
    case m15 = "15m"
    case m30 = "30m"
    case h1 = "1h"
    case h4 = "4h"
    case d1 = "1d"
    case w1 = "1w"

    public var seconds: Int {
        switch self {
        case .m1: 60
        case .m5: 300
        case .m15: 900
        case .m30: 1_800
        case .h1: 3_600
        case .h4: 14_400
        case .d1: 86_400
        case .w1: 604_800
        }
    }

    public var label: String { rawValue }

    public init?(seconds: Int) {
        guard let match = Self.allCases.first(where: { $0.seconds == seconds }) else { return nil }
        self = match
    }
}

extension ChartInterval {
    public static func `default`(for range: AssetChartRange) -> ChartInterval {
        switch range {
        case .oneHour: .m1
        case .oneDay: .m5
        case .oneWeek, .oneMonth, .threeMonths: .h1
        case .sixMonths, .oneYear, .all: .d1
        }
    }

    public static func preferred(for range: AssetChartRange) -> ChartInterval {
        switch range {
        case .oneHour: .m1
        case .oneDay: .m15
        case .oneWeek: .h1
        case .oneMonth: .h4
        case .threeMonths: .d1
        case .sixMonths, .oneYear, .all: .w1
        }
    }

    public static func allowed(for range: AssetChartRange) -> [ChartInterval] {
        switch range {
        case .oneHour: [.m1, .m5]
        case .oneDay, .oneWeek: [.m5, .m15, .m30, .h1, .h4, .d1]
        case .oneMonth: [.m15, .m30, .h1, .h4, .d1]
        case .threeMonths: [.m30, .h1, .h4, .d1, .w1]
        case .sixMonths: [.h1, .h4, .d1, .w1]
        case .oneYear, .all: [.h4, .d1, .w1]
        }
    }
}
