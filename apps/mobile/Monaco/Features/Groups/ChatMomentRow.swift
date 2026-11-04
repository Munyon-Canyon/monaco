import MonacoCore
import SwiftUI

enum ChatMomentMetrics {
    static let widthFraction: CGFloat = 0.85
    static let logoSize: CGFloat = 28
    static let minHeight: CGFloat = 44
    static let eventLineInset: CGFloat = 4
    static let eventLineGap: CGFloat = 4
    static let badgeSpace: CGFloat = 24
}

struct ChatMomentRow: View {
    let moment: ChatMoment
    let lines: ChatMoment.Lines
    let spokenLabel: String
    let identifier: String
    var open: (() -> Void)?

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.displayScale) private var displayScale
    @MonacoReduceTransparency private var reduceTransparency

    var body: some View {
        switch moment {
        case .tradeFilled(let trade):
            pill(trade)
        case .cabalCreated, .memberJoined:
            eventLine
        }
    }

    private func pill(_ trade: ChatMoment.Trade) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            StockMark(symbol: trade.symbol, size: ChatMomentMetrics.logoSize)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 0) {
                Text(lines.title)
                    .font(Self.titleFont)
                    .foregroundStyle(MonacoTheme.primaryText)
                if lines.detail != nil { subtitle }
            }
            .multilineTextAlignment(.leading)
            if Self.fillsWidth(dynamicTypeSize.isAccessibilitySize) { Spacer(minLength: 0) }
            Image(systemName: "chevron.right")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.tertiaryText)
                .accessibilityHidden(true)
        }
        .padding(.leading, MonacoTheme.Space.s)
        .padding(.trailing, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: ChatMomentMetrics.minHeight)
        .modifier(MomentPillSurface(solid: reduceTransparency, displayScale: displayScale))
        .contentShape(Capsule())
        .onTapGesture { open?() }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spokenLabel)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier(identifier)
    }

    static let titleFont = Font.footnote.weight(.semibold)

    @ViewBuilder private var subtitle: some View {
        if let detail = lines.detail {
            if let change = lines.change {
                Text(detail).foregroundStyle(MonacoTheme.secondaryText)
                    + Text(", ").foregroundStyle(MonacoTheme.secondaryText)
                    + Text(change).foregroundStyle(Self.changeColor(lines.direction))
            } else {
                Text(detail).foregroundStyle(MonacoTheme.secondaryText)
            }
        }
    }

    private var eventLine: some View {
        Text(lines.title)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .multilineTextAlignment(.center)
            .padding(.vertical, ChatMomentMetrics.eventLineInset)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(spokenLabel)
            .accessibilityIdentifier(identifier)
    }

    static func fillsWidth(_ accessibilitySize: Bool) -> Bool { accessibilitySize }

    static func changeColor(_ direction: ChatMoment.Direction?) -> Color {
        switch direction {
        case .up: MonacoTheme.gain
        case .down: MonacoTheme.loss
        case .flat, nil: MonacoTheme.secondaryText
        }
    }

    static func identifier(rowID: String) -> String { "chat-moment-\(rowID)" }

    static func isEventLine(_ moment: ChatMoment) -> Bool {
        if case .tradeFilled = moment { return false }
        return true
    }

    enum Destination: Equatable {
        case proposal(String)
        case holding(symbol: String)
    }

    static func destination(of moment: ChatMoment) -> Destination? {
        guard case .tradeFilled(let trade) = moment else { return nil }
        if let proposalID = trade.proposalID { return .proposal(proposalID) }
        return .holding(symbol: trade.symbol)
    }

    static func topGap(previousIsEventLine: Bool) -> CGFloat {
        previousIsEventLine ? ChatMomentMetrics.eventLineGap : ChatBubbleMetrics.groupGap
    }

    static func spokenLabel(
        for moment: ChatMoment, lines: ChatMoment.Lines, now: Date, calendar: Calendar = .current,
        locale: Locale = .current
    ) -> String {
        guard case .tradeFilled(let trade) = moment else { return lines.title }
        var parts = [lines.title, "Filled \(spokenDate(trade.filledAt, now: now, calendar: calendar, locale: locale))"]
        if let detail = lines.detail, let now = detail.range(of: " · now ") {
            parts[1] += ", now \(detail[now.upperBound...])"
        }
        if let change = lines.change {
            let magnitude = change.trimmingCharacters(in: CharacterSet(charactersIn: "+-−–%"))
            switch lines.direction {
            case .up: parts[1] += ", up \(magnitude) percent"
            case .down: parts[1] += ", down \(magnitude) percent"
            case .flat, nil: parts[1] += ", unchanged"
            }
        }
        return parts.joined(separator: ". ") + ". Opens the trade."
    }

    private static func spokenDate(_ date: Date, now: Date, calendar: Calendar, locale: Locale) -> String {
        let sameYear = calendar.component(.year, from: date) == calendar.component(.year, from: now)
        return SharedFormatters.string(
            from: date, pattern: .fixed(sameYear ? "MMMM d" : "MMMM d, yyyy"), locale: locale, calendar: calendar)
    }
}

private struct MomentPillSurface: ViewModifier {
    let solid: Bool
    let displayScale: CGFloat

    @ViewBuilder
    func body(content: Content) -> some View {
        if solid {
            fill(content)
        } else if #available(iOS 26, *) {
            content.glassEffect(.regular, in: .capsule)
        } else {
            fill(content)
        }
    }

    private func fill(_ content: Content) -> some View {
        content
            .background(Capsule().fill(MonacoTheme.surfaceSunken))
            .overlay(Capsule().strokeBorder(MonacoTheme.hairline, lineWidth: 1 / displayScale))
    }
}
