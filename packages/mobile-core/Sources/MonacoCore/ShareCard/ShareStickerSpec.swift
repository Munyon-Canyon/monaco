import Foundation

public struct ShareStickerSpec: Equatable, Sendable {
    public struct Ticker: Equatable, Sendable {
        public let logoURL: URL?
        public let symbol: String
        public let detail: String?

        public init(logoURL: URL?, symbol: String, detail: String?) {
            self.logoURL = logoURL
            self.symbol = symbol
            self.detail = detail
        }
    }

    public let mark: ShareMark
    public let name: String
    public let ticker: Ticker?
    public let figureMain: String
    public let figureSecondary: String?
    public let tone: ShareFigureTone
    public let code: String?
    public let spoken: String

    public init(from spec: ShareCardSpec, eyes: ShareEyes, timeZone: TimeZone = .current) {
        mark = spec.header.mark
        name = spec.header.title
        ticker = spec.facts.symbol.map {
            Ticker(
                logoURL: spec.tickerLine.logoURL, symbol: $0,
                detail: eyes.buySell ? Self.since(spec.facts, timeZone: timeZone) : nil)
        }
        let percent = spec.figure.secondary ?? spec.figure.main
        figureMain = eyes.pnlDollars ? spec.figure.main : percent
        figureSecondary = eyes.pnlDollars ? spec.figure.secondary : nil
        tone = spec.figure.tone
        code = eyes.code ? spec.code : nil
        spoken = spec.spoken
    }

    private static func since(_ facts: ShareFacts, timeZone: TimeZone) -> String? {
        guard let first = facts.firstBoughtAt else { return nil }
        let day = SharedFormatters.string(
            from: first, pattern: .fixed("MMM d"), locale: ShareProposalText.posix,
            calendar: ShareProposalText.calendar(timeZone))
        return "\(facts.side == .short ? "short" : "long") since \(day)"
    }
}
