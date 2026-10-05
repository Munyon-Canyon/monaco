import Foundation

/// Which window of the US equities trading day the market is in.
///
/// The backend computes this from the exchange calendar, so the app never has to
/// guess from a clock or a time zone. `unknown` exists only so a session name the
/// server adds later cannot fail decoding of the whole response.
public enum MarketSession: String, Codable, Sendable, CaseIterable {
    case preMarket = "pre_market"
    case open
    case afterHours = "after_hours"
    case closed
    case unknown

    public init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = MarketSession(rawValue: raw) ?? .unknown
    }

    /// True only during the regular cash session.
    public var isRegularSession: Bool { self == .open }
}

/// An RFC3339 timestamp as the backend writes it.
///
/// The market payloads mix timestamps with plain numbers, and the asset routes are
/// decoded with a plain `JSONDecoder`, so the parsing lives on the field rather
/// than on the decoder's `dateDecodingStrategy`. It reuses the shared parser, which
/// already accepts both the fractional and whole-second spellings Go emits.
public struct MonacoTimestamp: Codable, Equatable, Sendable {
    public let date: Date

    public init(date: Date) {
        self.date = date
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        let raw = try container.decode(String.self)
        guard let parsed = SharedFormatters.iso8601Date(from: raw) else {
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "Expected ISO8601 date, got \(raw)"
            )
        }
        date = parsed
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        try container.encode(SharedFormatters.iso8601WholeSeconds.string(from: date))
    }
}
