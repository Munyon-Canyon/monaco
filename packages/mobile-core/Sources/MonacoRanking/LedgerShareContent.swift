import Foundation
import MonacoAPI

public enum LedgerShareMarkerKind: Equatable, Sendable { case call, buy }

public struct LedgerShareContent: Equatable, Sendable {
    public struct Cover: Equatable, Sendable {
        public let cabalID: String
        public let name: String
        public let pictureURL: URL?
    }

    public struct Point: Equatable, Sendable {
        public let time: Date
        public let returnBps: Int64
    }

    public struct Marker: Equatable, Sendable {
        public let kind: LedgerShareMarkerKind
        public let time: Date
        public let label: String
    }

    public static let figureContext = "this week"
    public static let replayTitle = "Replay the week"
    public static let joinLine = "Join with code"
    static let weekWord = "Week"

    public let weekTitle: String
    public let header: String
    public let subline: String
    public let figure: String
    public let isLoss: Bool
    public let bestCall: String
    public let sharpest: String
    public let voted: String
    public let code: String?
    public let cover: Cover
    public let photoURL: URL?
    public let name: String
    public let points: [Point]
    public let markers: [Marker]
    public let spokenReturn: String?
    public let spokenBestCall: String?

    public init(
        _ ledger: Components.Schemas.Ledger, name: String, photoURL: URL?, code: String?,
        timeZone: TimeZone
    ) {
        let weekTitle = Self.weekTitle(ledger.week)
        self.weekTitle = weekTitle
        self.name = name
        self.photoURL = photoURL
        header = "\(name) \u{00B7} \(weekTitle)"
        subline = Self.subline(ledger, names: ledger.cabals.map(\.name))
        figure = LedgerCopy.hero(bps: ledger.stakeReturnBps)
        isLoss = LedgerCopy.heroTone(bps: ledger.stakeReturnBps) == .loss
        bestCall = ledger.bestCall.map { LedgerCopy.signedPercent(bps: Int($0.moveBps)) } ?? LedgerCopy.dash
        sharpest = LedgerCopy.sharpest(ledger.sharpest?.user.name)
        voted = LedgerCopy.ofTotal(Int(ledger.voted), Int(ledger.eligible))
        self.code = code.map { $0.uppercased() }
        cover = Self.cover(ledger.cabals, fallbackName: name)
        points = ledger.series.map { Point(time: $0.at, returnBps: $0.returnBps) }
        markers = ledger.markers.map { marker in
            Marker(
                kind: marker.kind == .call ? .call : .buy, time: marker.at,
                label: marker.kind == .call
                    ? Self.callLabel(symbol: marker.symbol, at: marker.at, timeZone: timeZone) : "B")
        }
        spokenReturn = ledger.stakeReturnBps.map { Self.spoken(bps: Int($0)) }
        spokenBestCall = ledger.bestCall.map { Self.spoken(bps: Int($0.moveBps)) }
    }

    public static func weekTitle(_ week: String) -> String {
        guard let number = week.split(separator: "W").last.flatMap({ Int($0) }) else { return "This week" }
        return "\(weekWord) \(number)"
    }

    public static func subline(_ ledger: Components.Schemas.Ledger, names: [String]) -> String {
        let shown = names.prefix(3).joined(separator: ", ")
        let extra = names.count - 3
        let cabals = extra > 0 ? "\(shown) and \(extra) more" : shown
        guard let range = LedgerCopy.range(start: ledger.labelStart, end: ledger.labelEnd) else { return cabals }
        return cabals.isEmpty ? range : "\(range) \u{00B7} \(cabals)"
    }

    public static func cover(_ cabals: [Components.Schemas.LedgerCabal], fallbackName: String) -> Cover {
        func url(_ cabal: Components.Schemas.LedgerCabal) -> URL? {
            cabal.pictureUrl.flatMap { $0.isEmpty ? nil : URL(string: $0) }
        }
        if let withPicture = cabals.first(where: { url($0) != nil }) {
            return Cover(cabalID: withPicture.id, name: withPicture.name, pictureURL: url(withPicture))
        }
        guard let first = cabals.first else { return Cover(cabalID: "", name: fallbackName, pictureURL: nil) }
        return Cover(cabalID: first.id, name: first.name, pictureURL: nil)
    }

    public static func callLabel(symbol: String, at time: Date, timeZone: TimeZone) -> String {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        let days = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]
        return "Called \(symbol) \u{00B7} \(days[calendar.component(.weekday, from: time) - 1])"
    }

    static func spoken(bps: Int) -> String {
        let tenths = LedgerCopy.tenths(bps)
        let value = tenths % 10 == 0 ? "\(tenths / 10)" : "\(tenths / 10).\(tenths % 10)"
        switch LedgerCopy.tone(bps: bps) {
        case .gain: return "up \(value) percent"
        case .loss: return "down \(value) percent"
        case .flat: return "flat"
        }
    }
}
