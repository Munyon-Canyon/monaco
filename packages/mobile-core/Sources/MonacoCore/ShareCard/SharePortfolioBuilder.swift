import Foundation

public struct SharePortfolio: Equatable, Sendable {
    public let name: String
    public let photoURL: URL?
    public let cabalCount: Int
    public let totalValueMicros: Int64
    public let pnl1mMicros: Int64
    public let return1mBps: Int64?
    public let rightCalls: Int?
    public let resolvedCalls: Int?

    public init(
        name: String, photoURL: URL?, cabalCount: Int, totalValueMicros: Int64, pnl1mMicros: Int64,
        return1mBps: Int64?, rightCalls: Int?, resolvedCalls: Int?
    ) {
        self.name = name
        self.photoURL = photoURL
        self.cabalCount = cabalCount
        self.totalValueMicros = totalValueMicros
        self.pnl1mMicros = pnl1mMicros
        self.return1mBps = return1mBps
        self.rightCalls = rightCalls
        self.resolvedCalls = resolvedCalls
    }
}

public enum SharePortfolioBuilder {
    public static func build(
        portfolio: SharePortfolio, history: [ShareChartPoint], code: String?, eyes: ShareEyes, now: Date,
        timeZone: TimeZone = .current
    ) -> ShareCardSpec {
        let calendar = ShareProposalText.calendar(timeZone)
        let cabals = portfolio.cabalCount == 1 ? "1 cabal" : "\(portfolio.cabalCount) cabals"
        let percent = ShareFigureText.percent(basisPoints: portfolio.return1mBps)
        let rights =
            portfolio.rightCalls.flatMap { right in portfolio.resolvedCalls.map { "\(right) of \($0)" } }
            ?? ShareProposalText.missing
        let title = portfolio.name
        return ShareCardSpec(
            header: .init(
                mark: .photo(url: portfolio.photoURL, name: portfolio.name, crest: nil), title: title,
                subtitle: cabals,
                dateText: SharedFormatters.string(
                    from: now, pattern: .fixed("MMM d, yyyy"), locale: ShareProposalText.posix, calendar: calendar)),
            chart: ShareChart(points: rebased(history), markers: []),
            tickerLine: .init(logoURL: nil, symbol: portfolio.name, detail: cabals),
            figure: ShareFigureText.figure(
                pnlMicros: portfolio.pnl1mMicros, basisPoints: portfolio.return1mBps, dollars: eyes.pnlDollars),
            stats: [
                .init(label: "Positions", value: ShareProposalText.wholeDollars(portfolio.totalValueMicros)),
                .init(label: "P&L \u{00B7} 1M", value: percent),
                .init(label: "Right calls", value: rights),
            ],
            code: eyes.code ? code?.uppercased() : nil,
            replayTitle: "Replay your month",
            facts: ShareFacts(),
            spoken:
                "\(title), \(cabals), past month, "
                + ShareFigureText.spoken(
                    pnlMicros: portfolio.pnl1mMicros, basisPoints: portfolio.return1mBps, dollars: eyes.pnlDollars))
    }

    private static func rebased(_ history: [ShareChartPoint]) -> [ShareChartPoint] {
        let sorted = history.sorted { $0.time < $1.time }
        guard let base = sorted.first?.priceMicros else { return [] }
        return sorted.map { ShareChartPoint(time: $0.time, priceMicros: $0.priceMicros - base) }
    }
}
