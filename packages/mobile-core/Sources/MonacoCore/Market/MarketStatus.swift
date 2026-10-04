import Foundation

public struct MarketStatus: Codable, Equatable, Sendable {
    public let session: MarketSession
    public let isOpen: Bool
    public let afterHours: Bool
    public let nextSession: MarketSession?
    public let nextTransition: Date?
    public let asOf: Date?
    public let holiday: String?
    public let earlyClose: Bool

    public init(
        session: MarketSession,
        isOpen: Bool,
        afterHours: Bool,
        nextSession: MarketSession? = nil,
        nextTransition: Date? = nil,
        asOf: Date? = nil,
        holiday: String? = nil,
        earlyClose: Bool = false
    ) {
        self.session = session
        self.isOpen = isOpen
        self.afterHours = afterHours
        self.nextSession = nextSession
        self.nextTransition = nextTransition
        self.asOf = asOf
        self.holiday = holiday
        self.earlyClose = earlyClose
    }

    private enum CodingKeys: String, CodingKey {
        case session, isOpen, afterHours, nextSession, nextTransition, asOf, holiday, earlyClose
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        session = try container.decodeIfPresent(MarketSession.self, forKey: .session) ?? .unknown
        isOpen = try container.decodeIfPresent(Bool.self, forKey: .isOpen) ?? false
        afterHours = try container.decodeIfPresent(Bool.self, forKey: .afterHours) ?? !isOpen
        nextSession = try container.decodeIfPresent(MarketSession.self, forKey: .nextSession)
        nextTransition = try container.decodeIfPresent(MonacoTimestamp.self, forKey: .nextTransition)?.date
        asOf = try container.decodeIfPresent(MonacoTimestamp.self, forKey: .asOf)?.date
        holiday = try container.decodeIfPresent(String.self, forKey: .holiday)
        earlyClose = try container.decodeIfPresent(Bool.self, forKey: .earlyClose) ?? false
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(session, forKey: .session)
        try container.encode(isOpen, forKey: .isOpen)
        try container.encode(afterHours, forKey: .afterHours)
        try container.encodeIfPresent(nextSession, forKey: .nextSession)
        try container.encodeIfPresent(nextTransition.map(MonacoTimestamp.init(date:)), forKey: .nextTransition)
        try container.encodeIfPresent(asOf.map(MonacoTimestamp.init(date:)), forKey: .asOf)
        try container.encodeIfPresent(holiday, forKey: .holiday)
        try container.encode(earlyClose, forKey: .earlyClose)
    }
}
