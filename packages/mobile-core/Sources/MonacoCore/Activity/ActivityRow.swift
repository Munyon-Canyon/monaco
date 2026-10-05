import Foundation
import MonacoAPI

public struct ActivityRow: Identifiable, Equatable, Sendable {
    public enum Kind: Equatable, Sendable {
        case buy
        case sell
        case fund
        case cashOut
    }

    public enum Status: Equatable, Sendable {
        case pending
        case confirmed
        case failed

        public var rowLabel: String? {
            switch self {
            case .pending: "Pending"
            case .confirmed: nil
            case .failed: "Failed"
            }
        }

        public var receiptLabel: String {
            switch self {
            case .pending: "Pending"
            case .confirmed: "Done"
            case .failed: "Failed"
            }
        }
    }

    public let id: String
    public let kind: Kind
    public let symbol: String?
    public let glyph: String
    public let title: String
    public internal(set) var status: Status
    public internal(set) var offersRetry: Bool
    public let age: String
    public let fullDate: String
    public let amount: String?
    public let assetLine: String?
    public let actorName: String?
    public let actorID: String?
    public let actorHandle: String?
    public let solscanURL: URL?

    public init(_ activity: Components.Schemas.CabalActivity, now: Date, timeZone: TimeZone = .current) {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        let sameYear = calendar.component(.year, from: activity.occurredAt) == calendar.component(.year, from: now)
        let headline = Self.headline(activity.kind, assetName: activity.asset?.name)
        self.id = activity.id
        self.kind = Kind(activity.kind)
        self.symbol = activity.asset?.symbol
        self.glyph = headline.glyph
        self.title = headline.title
        self.status = Status(activity.status)
        self.offersRetry = status == .failed && kind.isSwap
        self.age = Self.string(activity.occurredAt, sameYear ? "MMM d, h:mm a" : "MMM d, yyyy", calendar)
        self.fullDate = Self.string(activity.occurredAt, "MMM d, yyyy 'at' h:mm a", calendar)
        self.amount = activity.usdcMicros.map { UsdAmountFormatter.format(micros: $0) }
        self.assetLine = activity.asset.map { "\($0.name) · \(AssetSymbolFormatter.display($0.symbol))" }
        self.actorName = activity.actor.map { $0.displayName.isEmpty ? "@\($0.handle)" : $0.displayName }
        self.actorID = activity.actor?.userId
        self.actorHandle = activity.actor?.handle
        self.solscanURL = activity.txSignature.flatMap { URL(string: "https://solscan.io/tx/\($0)") }
    }

    private static func headline(
        _ kind: Components.Schemas.CabalActivity.KindPayload, assetName: String?
    ) -> (title: String, glyph: String) {
        switch kind {
        case .buy: (assetName.map { "Bought \($0)" } ?? "Bought", "arrow.down")
        case .sell: (assetName.map { "Sold \($0)" } ?? "Sold", "arrow.up")
        case .fund: ("Money added", "plus")
        case .cashOut: ("Cashed out", "arrow.down.left")
        }
    }

    private static func string(_ date: Date, _ pattern: String, _ calendar: Calendar) -> String {
        SharedFormatters.string(
            from: date, pattern: .fixed(pattern), locale: Locale(identifier: "en_US_POSIX"), calendar: calendar)
    }
}

extension ActivityRow.Status {
    fileprivate init(_ payload: Components.Schemas.CabalActivity.StatusPayload) {
        switch payload {
        case .pending: self = .pending
        case .confirmed: self = .confirmed
        case .failed: self = .failed
        }
    }
}

extension ActivityRow.Kind {
    public var isSwap: Bool { self == .buy || self == .sell }

    fileprivate init(_ payload: Components.Schemas.CabalActivity.KindPayload) {
        switch payload {
        case .buy: self = .buy
        case .sell: self = .sell
        case .fund: self = .fund
        case .cashOut: self = .cashOut
        }
    }
}
