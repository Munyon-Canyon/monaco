import Foundation
import MonacoAPI

public struct FeedPostFacts: Equatable, Sendable {
    public enum Lead: Equatable, Sendable {
        case person(name: String, photoURL: String?, userID: String)
        case cabal(name: String, pictureURL: String?, cabalID: String?)
    }

    public enum Trailing: Equatable, Sendable {
        case figure(Int64)
        case text(String)
    }

    public struct Object: Equatable, Sendable {
        public let symbol: String?
        public let logoURL: String?
        public let headline: String
        public let place: String?
        public let trailing: Trailing?
        public internal(set) var placeIsWarning = false

        public var showsLogo: Bool { symbol != nil }
    }

    public struct Tally: Equatable, Sendable {
        public let yes: Int
        public let no: Int
        public let voters: Int
        public let needed: Int
        public let statusLine: String

        public init(yes: Int, no: Int, voters: Int, needed: Int, statusLine: String) {
            self.yes = yes
            self.no = no
            self.voters = voters
            self.needed = needed
            self.statusLine = statusLine
        }
    }

    public struct ChartPoint: Equatable, Sendable {
        public let at: Date
        public let priceMicros: Int64

        public init(at: Date, priceMicros: Int64) {
            self.at = at
            self.priceMicros = priceMicros
        }
    }

    public struct ChartMarker: Equatable, Sendable {
        public let at: Date
        public let side: FeedSide
        public let priceMicros: Int64

        public init(at: Date, side: FeedSide, priceMicros: Int64) {
            self.at = at
            self.side = side
            self.priceMicros = priceMicros
        }
    }

    public enum Attachment: Equatable, Sendable {
        case tally(Tally)
        case quote(String)
        case result(sealName: String, sparkline: [ChartPoint], isLoss: Bool)
        case sale(points: [ChartPoint], markers: [ChartMarker], isLoss: Bool)
        case none
    }

    public let lead: Lead
    public let name: String
    public let verb: String
    public let age: String
    public let object: Object?
    public let attachment: Attachment
    public let accentCabalID: String?
    public let cabalName: String?
    public let companyName: String?
    public let spoken: String

    public init(_ item: Components.Schemas.FeedItem, now: Date) {
        let kind = FeedPostKind(item.kind)
        let age = Age(item.createdAt, now: now)
        let lead = Self.lead(of: item, kind: kind)
        let result = kind == .result ? ResultReading(item, now: now) : nil
        let verb = result?.verb ?? Self.verb(of: item, kind: kind)
        let object = result?.object ?? Self.object(of: item, kind: kind)
        let attachment = result?.attachment ?? Self.attachment(of: item, kind: kind, now: now)

        self.lead = lead
        self.name = lead.name
        self.verb = verb.value
        self.age = age.short
        self.object = object?.line
        self.attachment = attachment.value
        self.accentCabalID = item.cabalId
        self.cabalName = Self.text(item.cabalName)
        self.companyName = Self.text(item.assetName)
        self.spoken = Self.spoken(
            name: lead.name, verb: verb.spoken, object: object, age: age.spoken, attachment: attachment.spoken)
    }
}

extension FeedPostFacts.Lead {
    fileprivate var name: String {
        switch self {
        case .person(let name, _, _), .cabal(let name, _, _): name
        }
    }
}

extension FeedPostFacts {
    struct Spoken<Value> {
        let value: Value
        let spoken: String
    }

    struct Line {
        let line: Object
        let spoken: String
    }

    fileprivate struct Age {
        let short: String
        let spoken: String

        init(_ date: Date, now: Date) {
            let elapsed = Int(now.timeIntervalSince(date))
            switch elapsed {
            case ..<60: (short, spoken) = ("now", "just now")
            case ..<3600: (short, spoken) = Self.unit(elapsed / 60, "m", "minute")
            case ..<86_400: (short, spoken) = Self.unit(elapsed / 3600, "h", "hour")
            case ..<(7 * 86_400): (short, spoken) = Self.unit(elapsed / 86_400, "d", "day")
            default:
                let label = RelativeTimeFormatter.label(date: date, now: now)
                (short, spoken) = (label, "on \(label)")
            }
        }

        private static func unit(_ count: Int, _ short: String, _ word: String) -> (String, String) {
            ("\(count)\(short)", "\(count) \(word)\(count == 1 ? "" : "s") ago")
        }
    }

    static func text(_ raw: String?) -> String? {
        guard let trimmed = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty else { return nil }
        return trimmed
    }

    fileprivate static func lead(of item: Components.Schemas.FeedItem, kind: FeedPostKind) -> Lead {
        let cabal = Lead.cabal(
            name: text(item.cabalName) ?? "Monaco", pictureURL: text(item.cabalPictureUrl), cabalID: item.cabalId)
        if kind == .result, item.shape == .cabal { return cabal }
        guard let actorID = item.actorId else { return cabal }
        return .person(name: text(item.actorName) ?? "Someone", photoURL: text(item.actorPhotoUrl), userID: actorID)
    }

    fileprivate static func verb(of item: Components.Schemas.FeedItem, kind: FeedPostKind) -> Spoken<String> {
        switch kind {
        case .proposal: return Spoken(value: "proposed", spoken: "proposed")
        case .memberJoined: return Spoken(value: "joined", spoken: "joined")
        case .cabalCreated: return Spoken(value: "started", spoken: "started")
        case .reply:
            let order = [item.action.map(capitalized), item.symbol.map { AssetSymbolFormatter.display($0) }]
                .compactMap { $0 }.joined(separator: " ")
            let phrase = order.isEmpty ? "replied" : "on \(order)"
            return Spoken(value: phrase, spoken: phrase)
        case .trade, .threshold, .result, .other: return Spoken(value: "", spoken: "")
        }
    }

    fileprivate static func object(of item: Components.Schemas.FeedItem, kind: FeedPostKind) -> Line? {
        switch kind {
        case .proposal: return proposalObject(of: item)
        case .memberJoined, .cabalCreated:
            let cabal = text(item.cabalName) ?? item.title
            return Line(
                line: Object(symbol: nil, logoURL: nil, headline: cabal, place: nil, trailing: nil), spoken: cabal)
        case .reply: return nil
        case .trade, .threshold, .result, .other:
            let place = text(item.cabalName)
            let headline = Object(symbol: nil, logoURL: nil, headline: item.title, place: nil, trailing: nil)
            return Line(line: headline, spoken: [item.title, place].compactMap { $0 }.joined(separator: ", "))
        }
    }

    fileprivate static func proposalObject(of item: Components.Schemas.FeedItem) -> Line? {
        guard let symbol = text(item.symbol) else { return nil }
        let ticker = AssetSymbolFormatter.display(symbol, kind: item.assetKind == .preIpo ? .preIpo : .stock)
        let order = [item.action.map(capitalized), ticker].compactMap { $0 }.joined(separator: " ")
        let figure = amount(of: item)
        let place = text(item.cabalName)
        let trailing: Trailing? =
            item.assetKind == .preIpo ? .text("Pre-IPO") : item.changeBps.map { .figure($0) }
        let headline = [order, figure?.short].compactMap { $0 }.joined(separator: " ")
        var spoken = [order, figure?.spoken].compactMap { $0 }.joined(separator: ", ")
        if let place { spoken += ", in \(place)" }
        return Line(
            line: Object(
                symbol: symbol, logoURL: text(item.assetLogoUrl), headline: headline, place: place,
                trailing: trailing),
            spoken: spoken)
    }

    static func amount(of item: Components.Schemas.FeedItem) -> (short: String, spoken: String)? {
        if let micros = item.usdcMicros {
            return (wholeDollars(micros), spokenDollars(micros))
        }
        guard let units = item.tokenAmount, let decimals = item.tokenDecimals, units > 0,
            let quantity = TokenQuantityFormatter.quantity(fromAtomics: String(units), decimals: decimals)
        else { return nil }
        let label = TokenQuantityFormatter.label(quantity: quantity, kind: item.assetKind == .preIpo ? .preIpo : .stock)
        return (label, label)
    }

    fileprivate static func wholeDollars(_ micros: Int64) -> String {
        let formatted = UsdAmountFormatter.format(micros: micros)
        return formatted.hasSuffix(".00") ? String(formatted.dropLast(3)) : formatted
    }

    fileprivate static func spokenDollars(_ micros: Int64) -> String {
        let cents = (micros + 5_000) / 10_000
        let (dollars, remainder) = (cents / 100, cents % 100)
        guard remainder == 0 else { return "\(dollars).\(remainder < 10 ? "0" : "")\(remainder) dollars" }
        return "\(dollars) \(dollars == 1 ? "dollar" : "dollars")"
    }

    static func capitalized(_ word: String) -> String {
        word.prefix(1).uppercased() + word.dropFirst()
    }

    fileprivate static func attachment(
        of item: Components.Schemas.FeedItem, kind: FeedPostKind, now: Date
    ) -> Spoken<Attachment> {
        switch kind {
        case .proposal:
            guard let tally = item.tally else { return Spoken(value: .none, spoken: "") }
            let status = status(of: item, voted: tally.yes + tally.no, voters: tally.voters, now: now)
            let facts = Tally(
                yes: tally.yes, no: tally.no, voters: tally.voters, needed: tally.needed, statusLine: status.line)
            return Spoken(
                value: .tally(facts), spoken: "\(tally.yes) yes, \(tally.no) no. \(status.spoken).")
        case .reply:
            guard let body = text(item.body) else { return Spoken(value: .none, spoken: "") }
            return Spoken(value: .quote("“\(body)”"), spoken: "“\(body)”")
        case .memberJoined, .cabalCreated, .trade, .threshold, .result, .other:
            return Spoken(value: .none, spoken: "")
        }
    }

    fileprivate static func status(
        of item: Components.Schemas.FeedItem, voted: Int, voters: Int, now: Date
    ) -> (line: String, spoken: String) {
        let count = "\(voted) of \(voters) voted"
        guard let outcome = closedOutcome(item.status) else {
            guard let expiresAt = item.expiresAt else { return (count, count) }
            let left = Countdown(expiresAt.timeIntervalSince(now))
            return ("\(count) · \(left.short)", "\(count), \(left.spoken)")
        }
        let tally = item.tally.map { "\($0.yes)–\($0.no)" }
        let head = [outcome, tally.flatMap { ["Passed", "Failed"].contains(outcome) ? $0 : nil }]
            .compactMap { $0 }.joined(separator: " ")
        return ("\(head) · \(count)", "\(head). \(count)")
    }

    fileprivate static func closedOutcome(_ status: String?) -> String? {
        switch status {
        case "passed", "executed", "execution_failed", "execution_blocked": "Passed"
        case "failed", "expired": "Failed"
        case "withdrawn": "Withdrawn"
        case "voided": "Voided"
        default: nil
        }
    }

    fileprivate struct Countdown {
        let short: String
        let spoken: String

        init(_ remaining: TimeInterval) {
            guard remaining > 0 else {
                (short, spoken) = ("ending now", "ending now")
                return
            }
            let minutes = Int((remaining / 60).rounded(.up))
            let hours = Int((remaining / 3600).rounded(.up))
            let days = Int((remaining / 86_400).rounded(.up))
            let (count, abbreviation, word) =
                minutes < 60 ? (minutes, "m", "minute") : hours < 24 ? (hours, "h", "hour") : (days, "d", "day")
            (short, spoken) = ("ends in \(count)\(abbreviation)", "ends in \(count) \(word)\(count == 1 ? "" : "s")")
        }
    }

    fileprivate static func spoken(
        name: String, verb: String, object: Line?, age: String, attachment: String
    ) -> String {
        var head = [name, verb].filter { !$0.isEmpty }.joined(separator: " ")
        if let object { head += " \(object.spoken)" }
        let sentence = "\(head), \(age)."
        return attachment.isEmpty ? sentence : "\(sentence) \(attachment)"
    }
}
