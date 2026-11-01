import Foundation
import MonacoAPI

enum ShareProposalText {
    static let missing = "\u{2014}"
    static let posix = Locale(identifier: "en_US_POSIX")

    static func calendar(_ timeZone: TimeZone) -> Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        return calendar
    }

    static func day(_ date: Date, calendar: Calendar) -> String {
        SharedFormatters.string(from: date, pattern: .fixed("MMM d"), locale: posix, calendar: calendar)
    }

    static func wholeDollars(_ micros: Int64) -> String {
        let text = UsdAmountFormatter.format(micros: (micros + 500_000) / 1_000_000 * 1_000_000)
        return text.hasSuffix(".00") ? String(text.dropLast(3)) : text
    }

    static func amountMicros(_ proposal: Components.Schemas.ProposalDetail) -> Int64? {
        proposal.kind == .sell ? proposal.quoteOutAmount : proposal.usdcMicros
    }

    static func amountText(_ proposal: Components.Schemas.ProposalDetail) -> String {
        amountMicros(proposal).map(wholeDollars) ?? missing
    }

    static func symbol(_ proposal: Components.Schemas.ProposalDetail) -> String {
        AssetSymbolFormatter.display(proposal.symbol ?? "")
    }

    static func sharesText(atomics: Int64?, decimals: Int) -> String? {
        guard let atomics else { return nil }
        let label = TokenQuantityFormatter.label(fromAtomics: String(atomics), decimals: decimals, kind: .stock)
        return label.replacingOccurrences(of: " shares", with: "").replacingOccurrences(of: " share", with: "")
    }

    static func price(_ micros: Int64?) -> String {
        micros.flatMap { $0 > 0 ? UsdAmountFormatter.format(micros: $0) : nil } ?? missing
    }

    static func proposerName(_ proposal: Components.Schemas.ProposalDetail) -> String {
        let first = CabalInviteCopy.firstName(proposal.proposer.displayName)
        return first.isEmpty ? "A member" : first
    }
}

public enum SharePassedBuilder {
    public static func isPassed(_ status: Components.Schemas.ProposalStatus) -> Bool {
        switch status {
        case .passed, .executed, .failed, .executionBlocked: true
        case .open, .expired, .withdrawn, .voided: false
        }
    }

    private struct Fill {
        let priceMicros: Int64
        let time: Date
    }

    private enum Outcome {
        case filled(Fill)
        case pending
        case failed
    }

    public static func build(
        proposal: Components.Schemas.ProposalDetail, crest: ShareCrest, proposerPhoto: URL?, chart: [ShareChartPoint],
        code: String?, eyes: ShareEyes, timeZone: TimeZone = .current
    ) -> ShareCardSpec {
        let calendar = ShareProposalText.calendar(timeZone)
        let outcome = outcome(of: proposal)
        let isSell = proposal.kind == .sell
        let symbol = ShareProposalText.symbol(proposal)
        let tally = "\(proposal.tally.yes)\u{2013}\(proposal.tally.no)"
        let name = ShareProposalText.proposerName(proposal)
        let title = ResultCopy.calledIt(name)
        let verb = isSell ? "Sell" : "Buy"
        let stats = proposal.stats
        let decimals = stats.tokenDecimals ?? 0
        var fill: Fill?
        if case .filled(let value) = outcome { fill = value }
        let headerDate = fill?.time ?? stats.closedAt ?? proposal.expiresAt
        let markers = eyes.buySell ? markers(proposal: proposal, fill: fill, isSell: isSell) : []
        let shares = ShareProposalText.sharesText(atomics: stats.sharesAtomics, decimals: decimals)
        let nowPrice = stats.priceMicros.flatMap { $0 > 0 ? $0 : nil }
        let figure = figure(
            proposal: proposal, outcome: outcome, tally: tally, nowPrice: nowPrice, dollars: eyes.pnlDollars,
            decimals: decimals)
        return ShareCardSpec(
            header: .init(
                mark: .photo(url: proposerPhoto, name: name, crest: crest), title: title,
                subtitle:
                    "\(crest.name) \u{00B7} \(verb) \(symbol) \(ShareProposalText.amountText(proposal)) \u{00B7} passed \(tally)",
                dateText: ShareProposalText.day(headerDate, calendar: calendar)),
            chart: ShareChart(points: points(chart, from: proposal.createdAt), markers: markers),
            tickerLine: .init(
                logoURL: nil, symbol: symbol, detail: fill != nil ? shares.map { "\($0) sh" } ?? "" : ""),
            figure: figure,
            stats: statLines(proposal: proposal, fill: fill, shares: shares, nowPrice: nowPrice),
            code: eyes.code ? code?.uppercased() : nil,
            replayTitle: "Replay the call",
            facts: ShareFacts(symbol: symbol, firstBoughtAt: fill?.time, side: isSell ? .short : .long),
            spoken: spokenText(
                title: title, crest: crest, verb: verb, symbol: symbol, proposal: proposal, figure: figure))
    }

    private static func spokenText(
        title: String, crest: ShareCrest, verb: String, symbol: String, proposal: Components.Schemas.ProposalDetail,
        figure: ShareCardSpec.Figure
    ) -> String {
        let tally = "passed \(proposal.tally.yes) to \(proposal.tally.no)"
        return "\(title), \(crest.name), \(verb.lowercased()) \(symbol), \(tally). \(spoken(figure))"
    }

    private static func outcome(of proposal: Components.Schemas.ProposalDetail) -> Outcome {
        if let swap = proposal.swap, swap.status == .confirmed, let price = swap.priceMicros, price > 0,
            let time = swap.filledAt
        {
            return .filled(Fill(priceMicros: price, time: time))
        }
        if proposal.status == .failed || proposal.status == .executionBlocked || proposal.swap?.status == .failed {
            return .failed
        }
        return .pending
    }

    private static func markers(
        proposal: Components.Schemas.ProposalDetail, fill: Fill?, isSell: Bool
    ) -> [ShareMarker] {
        var result: [ShareMarker] = []
        if let closedAt = proposal.stats.closedAt, let passed = proposal.passedPriceMicros, passed > 0 {
            result.append(
                ShareMarker(
                    kind: .seal, time: closedAt, priceMicros: passed,
                    callout: "Passed \u{00B7} \(UsdAmountFormatter.format(micros: passed))"))
        }
        if let fill {
            result.append(
                ShareMarker(
                    kind: isSell ? .sell : .buy, time: fill.time, priceMicros: fill.priceMicros,
                    callout: "Filled \u{00B7} \(UsdAmountFormatter.format(micros: fill.priceMicros))"))
        }
        return result
    }

    private static func points(_ chart: [ShareChartPoint], from start: Date) -> [ShareChartPoint] {
        let sorted = chart.sorted { $0.time < $1.time }
        let since = sorted.filter { $0.time >= start }
        return since.count >= 2 ? since : sorted
    }

    private static func figure(
        proposal: Components.Schemas.ProposalDetail, outcome: Outcome, tally: String, nowPrice: Int64?, dollars: Bool,
        decimals: Int
    ) -> ShareCardSpec.Figure {
        let isSell = proposal.kind == .sell
        switch outcome {
        case .pending:
            return .init(
                main: "Passed \(tally)", secondary: nil, context: isSell ? "selling now" : "buying now", tone: .gain)
        case .failed:
            return .init(
                main: "Passed \(tally)", secondary: nil, context: "the trade didn\u{2019}t go through", tone: .gain)
        case .filled(let fill):
            guard let bps = proposal.stats.sinceBps, let nowPrice else {
                return .init(
                    main: UsdAmountFormatter.format(micros: fill.priceMicros), secondary: nil,
                    context: isSell ? "sold at" : "filled at", tone: .gain)
            }
            let percent = PercentFormatter.format(basisPoints: bps, signed: true, fractionDigits: 1)
            let change = dollarChange(
                atomics: proposal.stats.sharesAtomics, decimals: decimals, from: fill.priceMicros, to: nowPrice)
            var amount = percent
            var secondary: String?
            if dollars, let change {
                amount = UsdAmountFormatter.format(signedMicros: change)
                secondary = percent
            }
            if isSell {
                let word = bps < 0 ? "Avoided" : "Missed"
                return .init(
                    main: bps == 0 ? amount : "\(word) \(amount)", secondary: secondary, context: "since the sale",
                    tone: bps == 0 ? .neutral : (bps < 0 ? .gain : .loss))
            }
            return .init(
                main: amount, secondary: secondary, context: "since the fill", tone: bps < 0 ? .loss : .gain)
        }
    }

    static func dollarChange(atomics: Int64?, decimals: Int, from fill: Int64, to now: Int64) -> Int64? {
        guard let atomics, atomics > 0, (0...19).contains(decimals) else { return nil }
        let divisor = (0..<decimals).reduce(UInt64(1)) { total, _ in total * 10 }
        let (high, low) = UInt64(atomics).multipliedFullWidth(by: (now - fill).magnitude)
        guard high < divisor else { return nil }
        let (quotient, remainder) = divisor.dividingFullWidth((high, low))
        let rounded = quotient + (remainder >= divisor - remainder ? 1 : 0)
        guard rounded <= UInt64(Int64.max) else { return nil }
        return now < fill ? -Int64(rounded) : Int64(rounded)
    }

    private static func statLines(
        proposal: Components.Schemas.ProposalDetail, fill: Fill?, shares: String?, nowPrice: Int64?
    ) -> [ShareCardSpec.Stat] {
        let isSell = proposal.kind == .sell
        let now = ShareProposalText.price(nowPrice)
        if let fill {
            return [
                .init(label: isSell ? "Sold" : "Bought", value: shares.map { "\($0) sh" } ?? ShareProposalText.missing),
                .init(label: "At", value: UsdAmountFormatter.format(micros: fill.priceMicros)),
                .init(label: "Now", value: now),
            ]
        }
        return [
            .init(label: "Amount", value: ShareProposalText.amountText(proposal)),
            .init(label: "Passed at", value: ShareProposalText.price(proposal.passedPriceMicros)),
            .init(label: "Now", value: now),
        ]
    }

    private static func spoken(_ figure: ShareCardSpec.Figure) -> String {
        [figure.main, figure.secondary, figure.context].compactMap { $0 }.joined(separator: ", ") + "."
    }
}
