import Foundation

public struct SharePosition: Equatable, Sendable {
    public let crest: ShareCrest
    public let symbol: String
    public let logoURL: URL?
    public let unitsText: String
    public let valueMicros: Int64
    public let costBasisMicros: Int64
    public let pnlMicros: Int64
    public let avgEntryMicros: Int64?
    public let priceMicros: Int64
    public let firstBoughtAt: Date?

    public init(
        crest: ShareCrest, symbol: String, logoURL: URL?, unitsText: String, valueMicros: Int64,
        costBasisMicros: Int64, pnlMicros: Int64, avgEntryMicros: Int64?, priceMicros: Int64, firstBoughtAt: Date?
    ) {
        self.crest = crest
        self.symbol = symbol
        self.logoURL = logoURL
        self.unitsText = unitsText
        self.valueMicros = valueMicros
        self.costBasisMicros = costBasisMicros
        self.pnlMicros = pnlMicros
        self.avgEntryMicros = avgEntryMicros
        self.priceMicros = priceMicros
        self.firstBoughtAt = firstBoughtAt
    }
}

public struct ShareTrade: Equatable, Sendable {
    public let isBuy: Bool
    public let time: Date
    public let priceMicros: Int64

    public init(isBuy: Bool, time: Date, priceMicros: Int64) {
        self.isBuy = isBuy
        self.time = time
        self.priceMicros = priceMicros
    }
}

public enum SharePositionBuilder {
    public static func build(
        position: SharePosition, trades: [ShareTrade], chart: [ShareChartPoint], code: String?, eyes: ShareEyes,
        truncated: Bool, now: Date, timeZone: TimeZone = .current
    ) -> ShareCardSpec {
        let ordered = trades.sorted { $0.time < $1.time }
        let calendar = Self.calendar(timeZone)
        let markers = markers(for: ordered, calendar: calendar)
        let start = truncated ? ordered.first?.time : (position.firstBoughtAt ?? ordered.first?.time)
        let units = unitsLabel(position.unitsText)
        let figure = figure(for: position, dollars: eyes.pnlDollars)
        let tickerDetail = "Long · \(units)"
        let title = position.crest.name
        return ShareCardSpec(
            header: .init(
                mark: .crest(position.crest), title: title,
                subtitle: "Holding · \(callsText(ordered.count, truncated: truncated))",
                dateText: SharedFormatters.string(
                    from: now, pattern: .fixed("MMM d, yyyy"), locale: posix, calendar: calendar)),
            chart: ShareChart(
                points: points(chart, from: start), markers: eyes.buySell ? markers : []),
            tickerLine: .init(logoURL: position.logoURL, symbol: position.symbol, detail: tickerDetail),
            figure: figure,
            stats: stats(for: position, dollars: eyes.pnlDollars),
            code: eyes.code ? code?.uppercased() : nil,
            replayTitle: "Replay the position",
            facts: ShareFacts(symbol: position.symbol, firstBoughtAt: position.firstBoughtAt, side: .long),
            spoken: spoken(title: title, position: position, units: units, dollars: eyes.pnlDollars))
    }

    private static let posix = Locale(identifier: "en_US_POSIX")

    private static func calendar(_ timeZone: TimeZone) -> Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        return calendar
    }

    private static func callsText(_ count: Int, truncated: Bool) -> String {
        if truncated { return "\(count)+ calls" }
        return count == 1 ? "1 call" : "\(count) calls"
    }

    private static func unitsLabel(_ unitsText: String) -> String {
        let trimmed = trimmedUnits(unitsText)
        return trimmed == "1" ? "1 share" : "\(trimmed) shares"
    }

    private static func trimmedUnits(_ text: String) -> String {
        guard text.contains(".") else { return text }
        var result = text
        while result.hasSuffix("0") { result.removeLast() }
        if result.hasSuffix(".") { result.removeLast() }
        return result.isEmpty ? "0" : result
    }

    private static func points(_ chart: [ShareChartPoint], from start: Date?) -> [ShareChartPoint] {
        let sorted = chart.sorted { $0.time < $1.time }
        guard let start else { return sorted }
        let since = sorted.filter { $0.time >= start }
        return since.count >= 2 ? since : sorted
    }

    private static func markers(for ordered: [ShareTrade], calendar: Calendar) -> [ShareMarker] {
        let lastSell = ordered.lastIndex { !$0.isBuy }
        let lastBuy = ordered.lastIndex { $0.isBuy }
        var called: Set<Int> = []
        if let lastSell {
            called.insert(lastSell)
            if let lastBuy, lastBuy > lastSell { called.insert(lastBuy) }
        } else if let lastBuy {
            called.insert(lastBuy)
        }
        return ordered.enumerated().map { index, trade in
            ShareMarker(
                kind: trade.isBuy ? .buy : .sell, time: trade.time, priceMicros: trade.priceMicros,
                callout: called.contains(index) ? calloutText(trade, calendar: calendar) : nil)
        }
    }

    private static func calloutText(_ trade: ShareTrade, calendar: Calendar) -> String {
        let date = SharedFormatters.string(
            from: trade.time, pattern: .fixed("MMM d"), locale: posix, calendar: calendar)
        return "\(date) · \(UsdAmountFormatter.format(micros: trade.priceMicros))"
    }

    private static func percentBasisPoints(_ position: SharePosition) -> Int64? {
        let cost = position.costBasisMicros
        guard cost > 0 else { return nil }
        let scaled = position.pnlMicros * 10_000
        let half = cost / 2
        return (scaled >= 0 ? scaled + half : scaled - half) / cost
    }

    private static func figure(for position: SharePosition, dollars: Bool) -> ShareCardSpec.Figure {
        let bps = percentBasisPoints(position) ?? 0
        let percent = PercentFormatter.format(basisPoints: bps, signed: true, fractionDigits: 1)
        let tone: ShareFigureTone = position.pnlMicros < 0 ? .loss : .gain
        if dollars {
            return .init(
                main: UsdAmountFormatter.format(signedMicros: position.pnlMicros), secondary: percent, context: nil,
                tone: tone)
        }
        return .init(main: percent, secondary: nil, context: nil, tone: tone)
    }

    private static func stats(for position: SharePosition, dollars: Bool) -> [ShareCardSpec.Stat] {
        let first: ShareCardSpec.Stat =
            dollars
            ? .init(label: "Size", value: wholeDollars(position.valueMicros))
            : .init(label: "Shares", value: trimmedUnits(position.unitsText))
        return [
            first,
            .init(
                label: "Avg. entry",
                value: position.avgEntryMicros.map { UsdAmountFormatter.format(micros: $0) }
                    ?? CabalHoldingCopy.missing),
            .init(label: "Current", value: UsdAmountFormatter.format(micros: position.priceMicros)),
        ]
    }

    private static func wholeDollars(_ micros: Int64) -> String {
        let text = UsdAmountFormatter.format(micros: (micros + 500_000) / 1_000_000 * 1_000_000)
        return text.hasSuffix(".00") ? String(text.dropLast(3)) : text
    }

    private static func spoken(title: String, position: SharePosition, units: String, dollars: Bool) -> String {
        let direction = position.pnlMicros < 0 ? "down" : "up"
        let bps = abs(percentBasisPoints(position) ?? 0)
        let tenths = (bps + 5) / 10
        let percent = tenths % 10 == 0 ? "\(tenths / 10)" : "\(tenths / 10).\(tenths % 10)"
        let move = "\(direction) \(percent) percent"
        let figure = dollars ? "\(direction) \(spokenDollars(abs(position.pnlMicros))), \(percent) percent" : move
        return "\(title), \(position.symbol), long \(units), \(figure)"
    }

    private static func spokenDollars(_ micros: Int64) -> String {
        let cents = (micros + 5_000) / 10_000
        let whole = UsdAmountFormatter.format(micros: cents / 100 * 1_000_000).dropFirst().dropLast(3)
        return cents % 100 == 0 ? "\(whole) dollars" : "\(whole) dollars \(cents % 100)"
    }
}
