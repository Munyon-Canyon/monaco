import Foundation

public enum CabalHeaderCopy {
    public static let botValue = "Off"
    public static let moveRange = "1M"

    public struct Stat: Equatable, Sendable {
        public let label: String
        public let value: String
    }

    public struct Move: Equatable, Sendable {
        public let percent: String
        public let direction: PortfolioSummary.Direction

        public var line: String { "\(percent) \u{00B7} \(CabalHeaderCopy.moveRange)" }
    }

    private static let posix = Locale(identifier: "en_US_POSIX")

    public static func subline(
        memberCount: Int32, createdAt: Date, role: String?, canVote: Bool, now: Date, calendar: Calendar = .current
    ) -> String {
        var parts = [CabalCopy.memberCount(memberCount), since(createdAt, now: now, calendar: calendar)]
        if let viewer = viewer(role: role, canVote: canVote) { parts.append(viewer) }
        return parts.joined(separator: " \u{00B7} ")
    }

    public static func since(_ createdAt: Date, now: Date, calendar: Calendar = .current) -> String {
        if calendar.isDate(createdAt, inSameDayAs: now) { return "started today" }
        let month = SharedFormatters.string(
            from: createdAt, pattern: .fixed("MMM yyyy"), locale: posix, calendar: calendar)
        return "since \(month)"
    }

    public static func viewer(role: String?, canVote: Bool) -> String? {
        guard let role else { return nil }
        if role == "creator" || role == "admin" { return "you are admin" }
        return canVote ? "you vote" : "you hold a share"
    }

    public static func stats(shareMicros: Int64, sliceBps: Int, openVotes: Int) -> [Stat] {
        [
            Stat(label: "Your share", value: wholeDollars(shareMicros)),
            Stat(label: "Of treasury", value: percent(basisPoints: sliceBps)),
            Stat(label: "Open votes", value: "\(openVotes)"),
            Stat(label: "Bot", value: botValue),
        ]
    }

    public static func move(_ curve: ValueCurve?, createdAt: Date, now: Date, calendar: Calendar = .current) -> Move? {
        guard let monthAgo = calendar.date(byAdding: .month, value: -1, to: now), createdAt <= monthAgo,
            let points = curve?.navPoints, points.count >= 2, let first = points.first?.value, first > 0,
            let last = points.last?.value
        else { return nil }
        let delta = last - first
        let scaled = delta * 10_000
        let half = first / 2
        let basisPoints = (scaled >= 0 ? scaled + half : scaled - half) / first
        return Move(
            percent: PercentFormatter.format(basisPoints: basisPoints, signed: true, fractionDigits: 1),
            direction: PortfolioSummary.Direction(signedMicros: delta))
    }

    public static func compactDollars(_ micros: Int64) -> String {
        let text = UsdAmountFormatter.format(micros: micros)
        return text.hasSuffix(".00") ? String(text.dropLast(3)) : text
    }

    public static func shortName(_ name: String) -> String {
        var words = name.split(separator: " ")
        while words.count > 1, let last = words.last, companySuffixes.contains(last.lowercased()) {
            words.removeLast()
        }
        return words.joined(separator: " ")
    }

    private static let companySuffixes: Set<String> = [
        "management", "partners", "group", "holdings", "llc", "inc", "co",
    ]

    static func wholeDollars(_ micros: Int64) -> String {
        let text = UsdAmountFormatter.format(micros: max(micros, 0) / 1_000_000 * 1_000_000)
        return text.hasSuffix(".00") ? String(text.dropLast(3)) : text
    }

    static func percent(basisPoints: Int) -> String {
        if basisPoints > 0, basisPoints < 100 { return "<1%" }
        return "\((max(basisPoints, 0) + 50) / 100)%"
    }
}
