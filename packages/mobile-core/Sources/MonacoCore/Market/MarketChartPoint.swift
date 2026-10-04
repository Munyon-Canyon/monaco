import Foundation

public struct MarketChartPoint: Codable, Equatable, Sendable, Identifiable {
    public let timestamp: Int64
    public let priceUsdcMicros: Int64
    public let openUsdcMicros: Int64
    public let highUsdcMicros: Int64
    public let lowUsdcMicros: Int64

    public var id: Int64 { timestamp }
    public var date: Date { Date(timeIntervalSince1970: TimeInterval(timestamp)) }
    public var chartValue: Double {
        NSDecimalNumber(value: priceUsdcMicros).dividing(by: NSDecimalNumber(value: 1_000_000)).doubleValue
    }
    public var hasCandle: Bool { openUsdcMicros > 0 && highUsdcMicros > 0 && lowUsdcMicros > 0 }

    public init(
        timestamp: Int64,
        priceUsdcMicros: Int64,
        openUsdcMicros: Int64 = 0,
        highUsdcMicros: Int64 = 0,
        lowUsdcMicros: Int64 = 0
    ) {
        self.timestamp = timestamp
        self.priceUsdcMicros = priceUsdcMicros
        self.openUsdcMicros = openUsdcMicros
        self.highUsdcMicros = highUsdcMicros
        self.lowUsdcMicros = lowUsdcMicros
    }

    private enum CodingKeys: String, CodingKey {
        case timestamp, priceUsdcMicros, openUsdcMicros, highUsdcMicros, lowUsdcMicros
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        timestamp = try container.decode(Int64.self, forKey: .timestamp)
        priceUsdcMicros = try container.decode(Int64.self, forKey: .priceUsdcMicros)
        openUsdcMicros = try container.decodeIfPresent(Int64.self, forKey: .openUsdcMicros) ?? 0
        highUsdcMicros = try container.decodeIfPresent(Int64.self, forKey: .highUsdcMicros) ?? 0
        lowUsdcMicros = try container.decodeIfPresent(Int64.self, forKey: .lowUsdcMicros) ?? 0
    }
}

public enum MarketChartSource: String, Codable, Sendable {
    case benchmarks
    case hermes
    case unknown

    public init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = MarketChartSource(rawValue: raw) ?? .unknown
    }
}

public enum MarketPriceBasis: String, Codable, Sendable {
    case underlying
    case token
    case unknown

    public init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = MarketPriceBasis(rawValue: raw) ?? .unknown
    }
}
