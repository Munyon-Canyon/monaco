import SwiftUI

enum CabalCoverCardMetrics {
    static let railWidth: CGFloat = 300
    static let railHeight: CGFloat = 218
    static let listHeight: CGFloat = 208
    static let railLargeTextHeight: CGFloat = 330
    static let listLargeTextHeight: CGFloat = 260
    static let footerHeight: CGFloat = 44
    static let chevronSize: CGFloat = 13
    static let pressScale: CGFloat = 0.97
    static let nameLinesAtLargeText = 2
    static let skeletonBarHeight: CGFloat = 14
    static let skeletonNameWidth: CGFloat = 125
    static let skeletonFigureWidth: CGFloat = 68
}

struct CabalCoverCardModel: Equatable, Identifiable {
    struct Figure: Equatable {
        let amountText: String
        let percentText: String
        let direction: PnLTone
    }

    struct GroupReturn: Equatable {
        let periodText: String
        let percentText: String
        let direction: PnLTone
    }

    let id: String
    let name: String
    let memberCountText: String
    var votesNeededText: String?
    var shareText: String?
    var ownershipText: String?
    var pnl: Figure?
    var pnlUnavailableReason: String?
    var groupReturn: GroupReturn?
    let tint: MonacoTheme.CabalTint
    var pictureURL: String?

    var spokenLabel: String {
        var parts = [name, memberCountText]
        if let votesNeededText { parts.append(votesNeededText) }
        if let shareText {
            parts.append("your share \(shareText)")
            if let ownershipText { parts.append("ownership \(ownershipText)") }
        } else {
            parts.append("no share")
        }
        if let pnl {
            parts.append(
                "your all-time P&L \(PnLSpeech.dollars(pnl.amountText)), \(PnLSpeech.percent(pnl.percentText))")
        } else if let pnlUnavailableReason {
            parts.append("your all-time P&L \(pnlUnavailableReason.lowercased())")
        }
        if let groupReturn {
            parts.append(
                "cabal \(Self.spokenPeriod(groupReturn.periodText)) \(PnLSpeech.percent(groupReturn.percentText))")
        }
        return parts.joined(separator: ", ")
    }

    private static func spokenPeriod(_ text: String) -> String {
        switch text {
        case "1W": return "1 week"
        case "1M": return "1 month"
        case "3M": return "3 months"
        case "All": return "all time"
        default: return text
        }
    }
}

enum CabalCoverCardLayout: Equatable {
    case rail, list

    var width: CGFloat? {
        switch self {
        case .rail: return CabalCoverCardMetrics.railWidth
        case .list: return nil
        }
    }

    func minHeight(largeText: Bool) -> CGFloat {
        switch self {
        case .rail: return largeText ? CabalCoverCardMetrics.railLargeTextHeight : CabalCoverCardMetrics.railHeight
        case .list: return largeText ? CabalCoverCardMetrics.listLargeTextHeight : CabalCoverCardMetrics.listHeight
        }
    }
}

struct CabalCoverCard: View {
    let model: CabalCoverCardModel
    var layout: CabalCoverCardLayout = .rail
    let onOpen: () -> Void
    let onShare: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .footnote) private var chevronSize = CabalCoverCardMetrics.chevronSize

    private var isLargeText: Bool { dynamicTypeSize.isAccessibilitySize }

    var body: some View {
        Button(action: onOpen) {
            CabalCover(
                cabalId: model.id, tint: model.tint, pictureURL: model.pictureURL.flatMap(URL.init(string:)),
                style: .memberCard
            ) {
                content
            }
            .frame(width: layout.width)
            .frame(maxWidth: layout.width == nil ? .infinity : nil)
            .frame(minHeight: layout.minHeight(largeText: isLargeText))
            .fixedSize(horizontal: false, vertical: true)
        }
        .buttonStyle(CabalCoverPressStyle())
        .accessibilityLabel(model.spokenLabel)
        .accessibilityIdentifier("cabal-cover-card-\(model.id)")
        .overlayPreferenceValue(ShareDiscAnchor.self) { anchor in
            GeometryReader { proxy in
                if let anchor {
                    let frame = proxy[anchor]
                    CoverShareDisc(cabalName: model.name, action: onShare)
                        .position(x: frame.midX, y: frame.midY)
                }
            }
        }
    }

    private var content: some View {
        VStack(alignment: .leading, spacing: 0) {
            head
            values.padding(.top, MonacoTheme.Space.s)
            MonacoRule(color: MonacoTheme.onCoverGlassStroke)
                .padding(.vertical, MonacoTheme.Space.sm)
            pnlRow
            Spacer(minLength: 0)
            footer
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.sm)
    }

    @ViewBuilder private var head: some View {
        if isLargeText {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                nameBlock
                badge
            }
        } else {
            HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
                nameBlock
                Spacer(minLength: MonacoTheme.Space.s)
                badge
            }
        }
    }

    private var nameBlock: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(model.name)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.onCover)
                .lineLimit(isLargeText ? CabalCoverCardMetrics.nameLinesAtLargeText : 1)
                .truncationMode(.tail)
            Text(model.memberCountText)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onCoverMuted)
        }
    }

    @ViewBuilder private var badge: some View {
        if let votes = model.votesNeededText {
            StatusBadge(votes, tone: .attention)
        }
    }

    @ViewBuilder private var values: some View {
        if isLargeText {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                shareValue(alignment: .leading)
                ownershipValue(alignment: .leading)
            }
        } else {
            HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
                shareValue(alignment: .leading)
                Spacer(minLength: MonacoTheme.Space.s)
                ownershipValue(alignment: .trailing)
            }
        }
    }

    private func shareValue(alignment: HorizontalAlignment) -> some View {
        VStack(alignment: alignment, spacing: 0) {
            Text("Your share")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onCoverMuted)
            figure(model.shareText, style: .card)
        }
    }

    private func ownershipValue(alignment: HorizontalAlignment) -> some View {
        VStack(alignment: alignment, spacing: 0) {
            Text("Ownership")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onCoverMuted)
            figure(model.ownershipText, style: .row)
        }
    }

    private func figure(_ text: String?, style: MoneyStyle) -> some View {
        Text(text ?? "—")
            .moneyFont(style)
            .foregroundStyle(text == nil ? MonacoTheme.onCoverMuted : MonacoTheme.onCover)
            .lineLimit(1)
            .minimumScaleFactor(style.minimumScaleFactor)
    }

    private var pnlRow: some View {
        Group {
            if isLargeText {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    pnlLabel("Your P&L · All time")
                    pnlFigures
                }
            } else {
                ViewThatFits(in: .horizontal) {
                    inlinePnlRow(label: "Your P&L · All time")
                    inlinePnlRow(label: "All-time P&L")
                }
            }
        }
    }

    private func inlinePnlRow(label: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
            pnlLabel(label).lineLimit(1)
            Spacer(minLength: MonacoTheme.Space.s)
            pnlFigures
        }
    }

    private func pnlLabel(_ text: String) -> some View {
        Text(text)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.onCoverMuted)
    }

    @ViewBuilder private var pnlFigures: some View {
        if let pnl = model.pnl {
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.xs) {
                Text(pnl.amountText)
                    .moneyFont(.row)
                    .foregroundStyle(pnl.direction.onCoverColor)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
                Text(pnl.percentText)
                    .moneyFont(.caption)
                    .foregroundStyle(pnl.direction.onCoverColor)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.caption.minimumScaleFactor)
            }
        } else {
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                Text("—")
                    .moneyFont(.row)
                    .foregroundStyle(MonacoTheme.onCoverMuted)
                if let reason = model.pnlUnavailableReason {
                    Text(reason)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.onCoverMuted)
                }
            }
        }
    }

    private var footer: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Color.clear
                .frame(width: CabalCoverCardMetrics.footerHeight, height: CabalCoverCardMetrics.footerHeight)
                .anchorPreference(key: ShareDiscAnchor.self, value: .bounds) { $0 }
            groupText
            Spacer(minLength: MonacoTheme.Space.s)
            Image(systemName: "chevron.right")
                .font(.system(size: chevronSize, weight: .semibold))
                .foregroundStyle(MonacoTheme.onCoverMuted)
        }
        .frame(minHeight: CabalCoverCardMetrics.footerHeight)
    }

    @ViewBuilder private var groupText: some View {
        if let trend = model.groupReturn {
            (Text("Cabal · \(trend.periodText) ")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onCoverMuted)
                + Text(trend.percentText)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(trend.direction.onCoverColor))
                .lineLimit(isLargeText ? nil : 1)
                .minimumScaleFactor(MoneyStyle.caption.minimumScaleFactor)
        }
    }
}

private struct ShareDiscAnchor: PreferenceKey {
    static let defaultValue: Anchor<CGRect>? = nil

    static func reduce(value: inout Anchor<CGRect>?, nextValue: () -> Anchor<CGRect>?) {
        value = value ?? nextValue()
    }
}

private struct CabalCoverPressStyle: ButtonStyle {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? CabalCoverCardMetrics.pressScale : 1)
            .animation(reduceMotion ? nil : .easeOut(duration: 0.15), value: configuration.isPressed)
    }
}

extension PnLTone {
    fileprivate var onCoverColor: Color {
        switch self {
        case .profit: return MonacoTheme.profitOnCover
        case .loss: return MonacoTheme.lossOnCover
        case .flat: return MonacoTheme.onCoverMuted
        }
    }
}

struct CabalCoverCardSkeleton: View {
    var layout: CabalCoverCardLayout = .rail

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
            .fill(MonacoTheme.surfaceSunken)
            .overlay(alignment: .topLeading) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    SkeletonBlock(
                        width: CabalCoverCardMetrics.skeletonNameWidth,
                        height: CabalCoverCardMetrics.skeletonBarHeight, radius: SkeletonMetrics.lineRadius)
                    SkeletonBlock(
                        width: CabalCoverCardMetrics.skeletonFigureWidth,
                        height: CabalCoverCardMetrics.skeletonBarHeight, radius: SkeletonMetrics.lineRadius)
                }
                .padding(.horizontal, MonacoTheme.Space.m)
                .padding(.vertical, MonacoTheme.Space.sm)
            }
            .frame(width: layout.width)
            .frame(maxWidth: layout.width == nil ? .infinity : nil)
            .frame(height: layout.minHeight(largeText: dynamicTypeSize.isAccessibilitySize))
            .accessibilityHidden(true)
    }
}

struct CabalCoverRail<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        ScrollView(.horizontal) {
            LazyHStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                content
            }
            .scrollTargetLayout()
        }
        .scrollTargetBehavior(.viewAligned)
        .contentMargins(.horizontal, MonacoTheme.Space.gutter)
        .scrollIndicators(.hidden)
    }
}
