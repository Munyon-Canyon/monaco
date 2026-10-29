import Foundation

public struct ProposalStats: Equatable, Sendable {
    public enum Tone: Equatable, Sendable { case plain, gain, loss }

    public struct Cell: Equatable, Sendable {
        public let label: String
        public let value: String
        public let tone: Tone
        public let hasInfo: Bool

        public init(label: String, value: String, tone: Tone = .plain, hasInfo: Bool = false) {
            self.label = label
            self.value = value
            self.tone = tone
            self.hasInfo = hasInfo
        }
    }

    public static let missing = "—"

    public let cells: [Cell]

    public init(cells: [Cell]) {
        self.cells = cells
    }

    public static func make(detail: ProposalDetail, asset: ProposalAsset?, now: Date, calendar: Calendar)
        -> ProposalStats
    {
        let summary = detail.summary
        let stats = detail.stats
        let swap = summary.swap
        let isSell = summary.kind == "sell"
        let estimate = asset?.kind == .preIpo ? asset?.estimateMicros : nil
        let priceCell = Cell(
            label: estimate == nil ? "Price" : "Estimate", value: money(estimate ?? stats.priceMicros),
            hasInfo: true)
        let sharesCell =
            estimate.flatMap { estimatedShares(usdcMicros: isSell ? nil : summary.usdcMicros, estimateMicros: $0) }
            ?? Cell(label: "Shares", value: sharesText(stats, exact: stats.sharesExact || isSell))
        let treasuryCell = Cell(label: "Of treasury", value: percent(stats.treasuryShareBps))

        if let swap, swap.status == "confirmed", swap.filledAt != nil, let fill = swap.priceMicros {
            return ProposalStats(cells: [
                Cell(label: "Fill", value: money(fill)),
                Cell(label: "Shares", value: sharesText(stats, exact: true)),
                Cell(label: "Now", value: money(stats.priceMicros)),
                since(stats.sinceBps),
            ])
        }

        switch summary.status {
        case .open:
            return ProposalStats(cells: [
                priceCell, sharesCell, treasuryCell,
                Cell(label: "Closes", value: when(stats.closesAt, now: now, calendar: calendar)),
            ])
        case .passed, .executed, .executionBlocked:
            return ProposalStats(cells: [
                priceCell, sharesCell, treasuryCell,
                Cell(label: "Passed", value: clock(stats.closedAt, calendar: calendar)),
            ])
        case .failed, .expired:
            return closed("Closed", priceCell, sharesCell, treasuryCell, stats, calendar)
        case .withdrawn:
            return closed("Withdrawn", priceCell, sharesCell, treasuryCell, stats, calendar)
        case .voided:
            return closed("Voided", priceCell, sharesCell, treasuryCell, stats, calendar)
        }
    }

    public static func crestAge(since created: Date, now: Date, calendar: Calendar) -> String {
        guard now.timeIntervalSince(created) >= 86_400 else {
            return ProposalCardCopy.age(since: created, now: now)
        }
        return dayLabel(created, calendar: calendar)
    }

    private static func closed(
        _ label: String, _ priceCell: Cell, _ sharesCell: Cell, _ treasuryCell: Cell, _ stats: ProposalStatsData,
        _ calendar: Calendar
    ) -> ProposalStats {
        ProposalStats(cells: [
            priceCell, sharesCell, treasuryCell,
            Cell(label: label, value: stats.closedAt.map { dayLabel($0, calendar: calendar) } ?? missing),
        ])
    }

    static func estimatedShares(usdcMicros: Int64?, estimateMicros: Int64) -> Cell? {
        guard let usdcMicros, usdcMicros > 0, estimateMicros > 0 else { return nil }
        let hundredths = (usdcMicros.multipliedReportingOverflow(by: 100))
        guard !hundredths.overflow else { return nil }
        let value = hundredths.partialValue / estimateMicros
        let fraction = value % 100
        return Cell(label: "Shares", value: "≈ \(value / 100).\(fraction < 10 ? "0" : "")\(fraction)")
    }

    private static func since(_ bps: Int64?) -> Cell {
        guard let bps else { return Cell(label: "Since", value: missing) }
        let tone: Tone = bps > 0 ? .gain : bps < 0 ? .loss : .plain
        let sign = bps > 0 ? "+" : bps < 0 ? typographicMinus : ""
        return Cell(label: "Since", value: "\(sign)\(figure(bps: bps.magnitude))%", tone: tone)
    }

    private static func money(_ micros: Int64?) -> String {
        micros.map { UsdAmountFormatter.format(micros: $0) } ?? missing
    }

    static func percent(_ bps: Int64?) -> String {
        guard let bps else { return missing }
        if bps > 0, bps < 10 { return "<0.1%" }
        return "\(figure(bps: bps.magnitude))%"
    }

    private static func figure(bps: UInt64) -> String {
        guard bps < 1_000 else { return "\((bps + 50) / 100)" }
        let tenths = (bps + 5) / 10
        return tenths % 10 == 0 ? "\(tenths / 10)" : "\(tenths / 10).\(tenths % 10)"
    }

    private static func sharesText(_ stats: ProposalStatsData, exact: Bool) -> String {
        guard let atomics = stats.sharesAtomics,
            let quantity = TokenQuantityFormatter.quantity(fromAtomics: String(atomics), decimals: stats.tokenDecimals)
        else { return missing }
        return sharesText(quantity, exact: exact)
    }

    static func sharesText(_ quantity: Decimal, exact: Bool) -> String {
        let formatted = sharesFormatter.string(from: quantity as NSDecimalNumber) ?? missing
        let body = formatted == "0" && quantity > 0 ? "<0.0001" : formatted
        return exact ? body : "≈ \(body)"
    }

    private static func when(_ date: Date, now: Date, calendar: Calendar) -> String {
        if calendar.isDate(date, inSameDayAs: now) { return clock(date, calendar: calendar) }
        let ahead = date.timeIntervalSince(now)
        if ahead > 0, ahead < 7 * 86_400 {
            let day = SharedFormatters.string(
                from: date, pattern: .fixed("EEE"), locale: posix, calendar: calendar)
            return "\(day) \(clock(date, calendar: calendar))"
        }
        return dayLabel(date, calendar: calendar)
    }

    private static func clock(_ date: Date?, calendar: Calendar) -> String {
        guard let date else { return missing }
        return SharedFormatters.string(from: date, pattern: .fixed("h:mm a"), locale: posix, calendar: calendar)
    }

    private static func dayLabel(_ date: Date, calendar: Calendar) -> String {
        SharedFormatters.string(from: date, pattern: .fixed("MMM d"), locale: posix, calendar: calendar)
    }

    private static let posix = Locale(identifier: "en_US_POSIX")

    private static let sharesFormatter: NumberFormatter = {
        let formatter = NumberFormatter()
        formatter.locale = posix
        formatter.numberStyle = .decimal
        formatter.usesGroupingSeparator = true
        formatter.groupingSeparator = ","
        formatter.minimumFractionDigits = 0
        formatter.maximumFractionDigits = 4
        formatter.roundingMode = .halfUp
        return formatter
    }()
}
