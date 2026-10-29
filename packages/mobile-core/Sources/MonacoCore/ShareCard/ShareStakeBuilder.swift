import Foundation

public struct ShareStake: Equatable, Sendable {
    public let crest: ShareCrest
    public let name: String
    public let photoURL: URL?
    public let ownershipBps: Int64
    public let valueMicros: Int64
    public let netContributedMicros: Int64
    public let pnlMicros: Int64
    public let firstFundedAt: Date?

    public init(
        crest: ShareCrest, name: String, photoURL: URL?, ownershipBps: Int64, valueMicros: Int64,
        netContributedMicros: Int64, pnlMicros: Int64, firstFundedAt: Date?
    ) {
        self.crest = crest
        self.name = name
        self.photoURL = photoURL
        self.ownershipBps = ownershipBps
        self.valueMicros = valueMicros
        self.netContributedMicros = netContributedMicros
        self.pnlMicros = pnlMicros
        self.firstFundedAt = firstFundedAt
    }
}

enum ShareFigureText {
    static func percent(basisPoints: Int64?) -> String {
        guard let basisPoints else { return ShareProposalText.missing }
        return PercentFormatter.format(basisPoints: basisPoints, signed: true, fractionDigits: 1)
    }

    static func figure(pnlMicros: Int64, basisPoints: Int64?, dollars: Bool) -> ShareCardSpec.Figure {
        let percent = percent(basisPoints: basisPoints)
        let tone: ShareFigureTone = pnlMicros < 0 ? .loss : .gain
        if dollars {
            return .init(
                main: UsdAmountFormatter.format(signedMicros: pnlMicros), secondary: percent, context: nil, tone: tone)
        }
        return .init(main: percent, secondary: nil, context: nil, tone: tone)
    }

    static func spoken(pnlMicros: Int64, basisPoints: Int64?, dollars: Bool) -> String {
        let direction = pnlMicros < 0 ? "down" : "up"
        let tenths = (abs(basisPoints ?? 0) + 5) / 10
        let percent = tenths % 10 == 0 ? "\(tenths / 10)" : "\(tenths / 10).\(tenths % 10)"
        guard dollars else { return "\(direction) \(percent) percent" }
        let cents = (abs(pnlMicros) + 5_000) / 10_000
        let whole = UsdAmountFormatter.format(micros: cents / 100 * 1_000_000).dropFirst().dropLast(3)
        let amount = cents % 100 == 0 ? "\(whole) dollars" : "\(whole) dollars \(cents % 100)"
        return "\(direction) \(amount), \(percent) percent"
    }
}

public enum ShareStakeBuilder {
    public static func build(
        stake: ShareStake, chart: [ShareChartPoint], code: String?, eyes: ShareEyes, timeZone: TimeZone = .current
    ) -> ShareCardSpec {
        let calendar = ShareProposalText.calendar(timeZone)
        let ownership = ownershipText(stake.ownershipBps)
        let since = stake.firstFundedAt.map {
            "Since "
                + SharedFormatters.string(
                    from: $0, pattern: .fixed("MMM yyyy"), locale: ShareProposalText.posix, calendar: calendar)
        }
        let returnBps = returnBasisPoints(stake)
        let title = stake.name
        return ShareCardSpec(
            header: .init(
                mark: .photo(url: stake.photoURL, name: stake.name, crest: stake.crest), title: title,
                subtitle: "\(ownership) of \(stake.crest.name)", dateText: since ?? ""),
            chart: ShareChart(points: points(chart, from: stake.firstFundedAt), markers: []),
            tickerLine: .init(logoURL: nil, symbol: "", detail: ""),
            figure: ShareFigureText.figure(
                pnlMicros: stake.pnlMicros, basisPoints: returnBps, dollars: eyes.pnlDollars),
            stats: [
                .init(label: "Share", value: ownership),
                .init(label: "Funded", value: ShareProposalText.wholeDollars(stake.netContributedMicros)),
                .init(label: "Now", value: ShareProposalText.wholeDollars(stake.valueMicros)),
            ],
            code: eyes.code ? code?.uppercased() : nil,
            replayTitle: "Replay your share",
            facts: ShareFacts(),
            spoken:
                "\(title), \(ownership) of \(stake.crest.name), "
                + ShareFigureText.spoken(
                    pnlMicros: stake.pnlMicros, basisPoints: returnBps, dollars: eyes.pnlDollars))
    }

    static func ownershipText(_ bps: Int64) -> String {
        PercentFormatter.format(basisPoints: bps, signed: false, fractionDigits: bps % 100 == 0 ? 0 : 1)
    }

    private static func returnBasisPoints(_ stake: ShareStake) -> Int64? {
        let cost = stake.netContributedMicros
        guard cost > 0 else { return nil }
        let scaled = stake.pnlMicros * 10_000
        let half = cost / 2
        return (scaled >= 0 ? scaled + half : scaled - half) / cost
    }

    private static func points(_ chart: [ShareChartPoint], from start: Date?) -> [ShareChartPoint] {
        let sorted = chart.sorted { $0.time < $1.time }
        guard let start else { return sorted }
        let since = sorted.filter { $0.time >= start }
        return since.count >= 2 ? since : sorted
    }
}
