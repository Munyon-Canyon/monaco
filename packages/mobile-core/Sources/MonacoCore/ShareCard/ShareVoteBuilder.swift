import Foundation
import MonacoAPI

public enum ShareVoteBuilder {
    public static func build(
        proposal: Components.Schemas.ProposalDetail, crest: ShareCrest, chart: [ShareChartPoint], code: String?,
        eyes: ShareEyes, now: Date, timeZone: TimeZone = .current
    ) -> ShareCardSpec {
        let calendar = ShareProposalText.calendar(timeZone)
        let symbol = ShareProposalText.symbol(proposal)
        let verb = proposal.kind == .sell ? "Sell" : "Buy"
        let tally = proposal.tally
        let amount = ShareProposalText.amountText(proposal)
        let share = proposal.stats.treasuryShareBps.map(treasuryText)
        let sorted = chart.sorted { $0.time < $1.time }
        let endsIn = ProposalVoteCopy.endsIn(proposal.expiresAt, now: now)
        let name = ShareProposalText.proposerName(proposal)
        let voted = "\(tally.yes + tally.no) of \(tally.voters) voted"
        let spoken =
            "\(crest.name), vote to \(verb.lowercased()) \(symbol) for \(amount), \(name) proposed, "
            + "\(tally.yes) yes, \(tally.no) no, \(voted)."
        return ShareCardSpec(
            header: .init(
                mark: .crest(crest), title: crest.name, subtitle: "\(name) proposed \u{00B7} \(endsIn)",
                dateText: ShareProposalText.day(now, calendar: calendar)),
            chart: ShareChart(points: sorted, markers: []),
            tickerLine: .init(logoURL: nil, symbol: symbol, detail: verb),
            figure: .init(main: amount, secondary: nil, context: share, tone: .neutral),
            stats: [
                .init(label: "Price", value: ShareProposalText.price(proposal.stats.priceMicros)),
                .init(label: "Votes", value: "\(tally.yes) yes \u{00B7} \(tally.no) no"),
                .init(label: "Voted", value: "\(tally.yes + tally.no) of \(tally.voters)"),
            ],
            code: eyes.code ? code?.uppercased() : nil,
            replayTitle: "Replay the price",
            facts: ShareFacts(symbol: symbol, firstBoughtAt: nil, side: proposal.kind == .sell ? .short : .long),
            spoken: spoken)
    }

    private static func treasuryText(_ bps: Int64) -> String {
        let text: String
        if bps >= 1_000 {
            text = "\((bps + 50) / 100)"
        } else {
            let tenths = (bps + 5) / 10
            text = tenths % 10 == 0 ? "\(tenths / 10)" : "\(tenths / 10).\(tenths % 10)"
        }
        return "\(text)% of treasury"
    }
}
