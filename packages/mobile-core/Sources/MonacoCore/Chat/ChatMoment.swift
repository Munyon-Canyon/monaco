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
        public let usdcMicros: Int64
        public let yes: Int?
        public let no: Int?
        public let proposer: Components.Schemas.ChatAuthor?
        public let proposerIsMe: Bool

        public var tally: (yes: Int, no: Int)? {
            guard let yes, let no else { return nil }
            return (yes, no)
        }
    }

    public enum SettledOutcome: Equatable, Sendable {
        case failed
        case expired
    }

    public struct Settled: Equatable, Sendable {
        public let outcome: SettledOutcome
        public let proposalID: String
        public let side: Side
        public let symbol: String
        public let usdcMicros: Int64
        public let tokenAmount: Int64?
        public let yes: Int
        public let no: Int
        public let voters: Int
        public let proposer: Components.Schemas.ChatAuthor?
        public let proposerIsMe: Bool
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
    case proposalSettled(Settled)

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
                    proposalID: fill.proposalId,
                    usdcMicros: fill.usdcMicros,
                    yes: fill.yes,
                    no: fill.no,
                    proposer: fill.proposer,
                    proposerIsMe: fill.proposer?.id == viewerID
                ))
        case .proposalSettled(let settled):
            self = .proposalSettled(
                Settled(
                    outcome: settled.outcome == .failed ? .failed : .expired,
                    proposalID: settled.proposalId,
                    side: settled.side == .buy ? .buy : .sell,
                    symbol: settled.symbol,
                    usdcMicros: settled.usdcMicros,
                    tokenAmount: settled.tokenAmount,
                    yes: settled.yes,
                    no: settled.no,
                    voters: settled.voters,
                    proposer: settled.proposer,
                    proposerIsMe: settled.proposer.id == viewerID
                ))
        }
    }

    public var symbol: String? {
        guard case .tradeFilled(let trade) = self else { return nil }
        return trade.symbol
    }

    public var proposalID: String? {
        switch self {
        case .tradeFilled(let trade): trade.proposalID
        case .proposalSettled(let settled): settled.proposalID
        case .cabalCreated, .memberJoined: nil
        }
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
            if let tally = trade.tally { return Self.lines(for: trade, tally: tally) }
            return Self.lines(
                for: trade, currentPriceMicros: currentPriceMicros, now: now, calendar: calendar, locale: locale)
        case .proposalSettled(let settled):
            return Self.lines(for: settled)
        }
    }

    public var voteSpokenLabel: String? {
        switch self {
        case .tradeFilled(let trade):
            guard let tally = trade.tally else { return nil }
            let price = Self.spokenDollars(trade.fillPriceMicros)
            let bought = Self.fillClause(trade)
            var sentences = ["Vote passed, \(tally.yes) to \(tally.no)", "\(bought.sentenceCased) at \(price)"]
            if let called = Self.calledIt(proposer: trade.proposer, isMe: trade.proposerIsMe) {
                sentences.append(called)
            }
            return sentences.joined(separator: ". ") + ". Opens the vote."
        case .proposalSettled(let settled):
            let nothing = settled.side == .buy ? "Nothing bought" : "Nothing sold"
            let head =
                settled.outcome == .failed
                ? "Vote failed, \(settled.yes) to \(settled.no). \(nothing)"
                : "Vote closed, \(settled.yes) to \(settled.no). Not enough votes. \(nothing)"
            let spokenHeadline = Self.headline(of: settled).replacingOccurrences(of: " \u{00B7} ", with: ", ")
            return "\(head). \(spokenHeadline). Opens the vote."
        case .cabalCreated, .memberJoined:
            return nil
        }
    }

    private static func firstName(of member: Components.Schemas.ChatAuthor) -> String {
        let name = member.displayName.split(separator: " ").first.map(String.init)
        return name ?? "Someone"
    }

    private static func lines(for trade: Trade, tally: (yes: Int, no: Int)) -> Lines {
        let price = "at \(UsdAmountFormatter.format(micros: trade.fillPriceMicros))"
        let detail = [price, calledIt(proposer: trade.proposer, isMe: trade.proposerIsMe)]
            .compactMap { $0 }.joined(separator: " \u{00B7} ")
        return Lines(
            title: "Passed \(tally.yes) to \(tally.no) \u{00B7} \(fillClause(trade))",
            detail: detail, change: nil, direction: nil)
    }

    private static func lines(for settled: Settled) -> Lines {
        let nothing = settled.side == .buy ? "nothing bought" : "nothing sold"
        let title =
            settled.outcome == .failed
            ? "Failed \(settled.yes) to \(settled.no) \u{00B7} \(nothing)"
            : "Closed \(settled.yes) to \(settled.no) \u{00B7} not enough votes"
        return Lines(title: title, detail: headline(of: settled), change: nil, direction: nil)
    }

    private static func headline(of settled: Settled) -> String {
        let ticker = AssetSymbolFormatter.display(settled.symbol)
        let name = "\(settled.side == .buy ? "Buy" : "Sell") \(ticker)"
        guard settled.usdcMicros > 0 else { return name }
        return "\(name) \u{00B7} \(CabalHeaderCopy.compactDollars(settled.usdcMicros))"
    }

    private static func calledIt(proposer: Components.Schemas.ChatAuthor?, isMe: Bool) -> String? {
        guard let proposer else { return nil }
        return ResultCopy.calledIt(isMe ? "You" : firstName(of: proposer))
    }

    private static func fillClause(_ trade: Trade) -> String {
        let verb = trade.side == .buy ? "bought" : "sold"
        let ticker = AssetSymbolFormatter.display(trade.symbol)
        guard let shares = shares(usdcMicros: trade.usdcMicros, priceMicros: trade.fillPriceMicros) else {
            return "\(verb) \(ticker)"
        }
        return "\(verb) \(shares) \(ticker)"
    }

    private static func shares(usdcMicros: Int64, priceMicros: Int64) -> String? {
        guard usdcMicros > 0, priceMicros > 0 else { return nil }
        var rounded = Decimal()
        var quotient = Decimal(usdcMicros) / Decimal(priceMicros)
        NSDecimalRound(&rounded, &quotient, 2, .plain)
        return sharesFormatter.string(from: rounded as NSDecimalNumber)
    }

    private static let sharesFormatter: NumberFormatter = {
        let formatter = NumberFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.numberStyle = .decimal
        formatter.usesGroupingSeparator = true
        formatter.groupingSeparator = ","
        formatter.minimumFractionDigits = 0
        formatter.maximumFractionDigits = 2
        return formatter
    }()

    private static func spokenDollars(_ micros: Int64) -> String {
        let cents = (micros + 5_000) / 10_000
        let dollars = cents / 100
        let remainder = cents % 100
        let unit = dollars == 1 ? "dollar" : "dollars"
        return remainder == 0 ? "\(dollars) \(unit)" : "\(dollars) \(unit) \(remainder)"
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

extension String {
    fileprivate var sentenceCased: String { prefix(1).uppercased() + dropFirst() }
}
