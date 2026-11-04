import Foundation
import MonacoAPI
import Observation

public enum ChatMoment: Equatable, Sendable {
    public enum Side: Equatable, Sendable {
        case buy
        case sell
    }

    public enum Direction: Equatable, Sendable {
        case up
        case down
        case flat
    }

    public struct Trade: Equatable, Sendable {
        public let side: Side
        public let symbol: String
        public let assetID: String
        public let fillPriceMicros: Int64
        public let filledAt: Date
        public let proposalID: String?
    }

    public struct Lines: Equatable, Sendable {
        public let title: String
        public let detail: String?
        public let change: String?
        public let direction: Direction?

        public var subtitle: String? {
            guard let detail else { return nil }
            guard let change else { return detail }
            return "\(detail), \(change)"
        }
    }

    case cabalCreated
    case memberJoined(member: Components.Schemas.ChatAuthor, isMe: Bool)
    case tradeFilled(Trade)

    public init?(message: ChatMessage, viewerID: String) {
        guard message.kind == .system, let moment = message.moment else { return nil }
        switch moment {
        case .cabalCreated:
            self = .cabalCreated
        case .memberJoined(let joined):
            self = .memberJoined(member: joined.member, isMe: joined.member.id == viewerID)
        case .tradeFilled(let fill):
            self = .tradeFilled(
                Trade(
                    side: fill.side == .buy ? .buy : .sell,
                    symbol: fill.symbol,
                    assetID: fill.assetId,
                    fillPriceMicros: fill.priceMicros,
                    filledAt: fill.filledAt,
                    proposalID: fill.proposalId
                ))
        }
    }

    public var symbol: String? {
        guard case .tradeFilled(let trade) = self else { return nil }
        return trade.symbol
    }

    public func lines(
        cabalName: String,
        currentPriceMicros: Int64?,
        now: Date,
        calendar: Calendar,
        locale: Locale
    ) -> Lines {
        switch self {
        case .cabalCreated:
            return Lines(title: "\(cabalName) was created", detail: nil, change: nil, direction: nil)
        case .memberJoined(let member, let isMe):
            let who = isMe ? "You" : Self.firstName(of: member)
            return Lines(title: "\(who) joined \(cabalName)", detail: nil, change: nil, direction: nil)
        case .tradeFilled(let trade):
            return Self.lines(
                for: trade, currentPriceMicros: currentPriceMicros, now: now, calendar: calendar, locale: locale)
        }
    }

    private static func firstName(of member: Components.Schemas.ChatAuthor) -> String {
        let name = member.displayName.split(separator: " ").first.map(String.init)
        return name ?? "Someone"
    }

    private static func lines(
        for trade: Trade, currentPriceMicros: Int64?, now: Date, calendar: Calendar, locale: Locale
    ) -> Lines {
        let verb = trade.side == .buy ? "Bought" : "Sold"
        let ticker = AssetSymbolFormatter.display(trade.symbol)
        let title = "\(verb) \(ticker) at \(UsdAmountFormatter.format(micros: trade.fillPriceMicros))"
        var detail = "Filled \(filledDate(trade.filledAt, now: now, calendar: calendar, locale: locale))"
        guard let current = currentPriceMicros else {
            return Lines(title: title, detail: detail, change: nil, direction: nil)
        }
        detail += " · now \(UsdAmountFormatter.format(micros: current))"
        guard trade.side == .buy, trade.fillPriceMicros > 0 else {
            return Lines(title: title, detail: detail, change: nil, direction: nil)
        }
        let scaled = (current - trade.fillPriceMicros) * 10_000
        let magnitude = (scaled.magnitude * 2 + UInt64(trade.fillPriceMicros)) / (UInt64(trade.fillPriceMicros) * 2)
        let basisPoints = scaled < 0 ? -Int64(magnitude) : Int64(magnitude)
        let direction: Direction = basisPoints > 0 ? .up : basisPoints < 0 ? .down : .flat
        return Lines(
            title: title,
            detail: detail,
            change: PercentFormatter.format(basisPoints: basisPoints, signed: true, fractionDigits: 1),
            direction: direction
        )
    }

    private static func filledDate(_ date: Date, now: Date, calendar: Calendar, locale: Locale) -> String {
        let sameYear = calendar.component(.year, from: date) == calendar.component(.year, from: now)
        return SharedFormatters.string(
            from: date,
            pattern: .fixed(sameYear ? "MMM d" : "MMM d, yyyy"),
            locale: locale,
            calendar: calendar
        )
    }
}

@Observable
@MainActor
public final class ChatMomentPrices {
    public static let chunkSize = 50

    private var prices: [String: Int64] = [:]
    @ObservationIgnored private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func price(for symbol: String) -> Int64? { prices[symbol] }

    public static func symbols(in rows: [ChatRow]) -> [String] {
        var seen: Set<String> = []
        return rows.compactMap { $0.moment?.symbol }.filter { seen.insert($0).inserted }
    }

    public func refresh(rows: [ChatRow]) async {
        let symbols = Self.symbols(in: rows)
        var start = 0
        while start < symbols.count {
            let chunk = Array(symbols[start..<min(start + Self.chunkSize, symbols.count)])
            start += Self.chunkSize
            let joined = chunk.joined(separator: ",")
            let list = try? await api.read {
                try await $0.getAssetQuotes(query: .init(symbols: joined)).ok.body.json
            }
            guard let list else { continue }
            for summary in list.assets {
                if let price = summary.priceMicros { prices[summary.symbol] = price }
            }
        }
    }
}
