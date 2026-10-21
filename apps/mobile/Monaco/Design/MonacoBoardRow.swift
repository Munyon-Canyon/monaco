import MonacoCore
import SwiftUI

enum BoardRowMetrics {
    static let rankWidth: CGFloat = 22
    static let minTrailingWidth: CGFloat = 80
    static let crownSize: CGFloat = 12
    static let movementGlyphSize: CGFloat = 10
    static let skeletonNameWidth: CGFloat = 125
    static let skeletonFigureWidth: CGFloat = 68
    static let skeletonBarHeight: CGFloat = 14

    static var avatarSize: CGFloat { MonacoAvatarSize.rank.rawValue }
}

struct BoardRow<Leading: View>: View {
    enum Figure {
        case pnl
        case value
        case returnPercent
    }

    static var viewerLabel: String { "You" }

    let row: LeaderboardRowView
    var figure: Figure = .pnl
    var isLast = false
    var chevron = false
    var movement: Int?
    var subtitle: String?
    @ViewBuilder let leading: Leading

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .body) private var titleWidthFloor = MonacoRowLayout.baseMinimumTitleWidth
    @ScaledMetric(relativeTo: .footnote) private var crownSize = BoardRowMetrics.crownSize
    @ScaledMetric(relativeTo: .footnote) private var movementGlyphSize = BoardRowMetrics.movementGlyphSize

    private var layout: MonacoRowLayout {
        MonacoRowLayout(dynamicTypeSize: dynamicTypeSize, scaledTitleWidthFloor: titleWidthFloor)
    }

    private var isLeader: Bool { row.rank == 1 }

    var body: some View {
        Group {
            if layout.isStacked {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    HStack(spacing: MonacoTheme.Space.sm) {
                        rankColumn
                        avatar
                        labels
                        if chevron { MonacoRowChevron() }
                    }
                    figures(alignment: .leading)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else {
                HStack(spacing: MonacoTheme.Space.sm) {
                    rankColumn
                    avatar
                    labels
                        .frame(minWidth: layout.minimumTitleWidth, alignment: .leading)
                    figures(alignment: .trailing)
                        .frame(minWidth: BoardRowMetrics.minTrailingWidth, alignment: .trailing)
                        .layoutPriority(1)
                    if chevron { MonacoRowChevron() }
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .background(row.isViewer ? MonacoTheme.brandWash : Color.clear)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.leading, ruleInset)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(Self.spoken(row, figure: figure, movement: movement))
    }

    private var avatar: some View {
        leading.frame(width: BoardRowMetrics.avatarSize, height: BoardRowMetrics.avatarSize)
    }

    private var ruleInset: CGFloat {
        guard !layout.isStacked else { return MonacoTheme.Space.gutter }
        return layout.separatorLeadingInset(markSize: BoardRowMetrics.avatarSize) + BoardRowMetrics.rankWidth
            + MonacoTheme.Space.sm
    }

    private func figures(alignment: HorizontalAlignment) -> some View {
        VStack(alignment: alignment, spacing: MonacoTheme.Space.xs) {
            switch figure {
            case .pnl:
                PnLText(signedMicros: row.pnlMicros, style: .row)
                returnText(.caption)
            case .value:
                MoneyText(micros: row.valueMicros, style: .row, color: MonacoTheme.primaryText)
                returnText(.caption)
            case .returnPercent:
                returnText(.row)
                PnLText(signedMicros: row.pnlMicros, style: .caption)
            }
        }
    }

    @ViewBuilder private func returnText(_ style: MoneyStyle) -> some View {
        if let bps = row.returnBps {
            PercentText(basisPoints: bps, style: style)
        } else {
            PercentText(percentReturn: nil, style: style)
        }
    }

    private var rankColumn: some View {
        VStack(spacing: 0) {
            if isLeader {
                Image(systemName: "crown.fill")
                    .font(.system(size: crownSize, weight: .semibold))
                    .foregroundStyle(MonacoTheme.goldGlyph)
            }
            Text("\(row.rank)")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(1)
            if let movement {
                movementMark(movement)
            }
        }
        .fixedSize()
        .frame(minWidth: BoardRowMetrics.rankWidth)
        .accessibilityHidden(true)
    }

    private func movementMark(_ movement: Int) -> some View {
        HStack(spacing: 0) {
            Image(systemName: Self.movementSymbol(movement))
                .font(.system(size: movementGlyphSize, weight: .bold))
            if movement != 0 {
                Text("\(abs(movement))")
                    .font(MonacoTheme.Typo.caption)
            }
        }
        .foregroundStyle(Self.movementColor(movement))
    }

    private var labels: some View {
        MonacoRowLabels(title: row.name, subtitle: subline, layout: layout) {
            if row.isViewer {
                Text(Self.viewerLabel)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.brandOnWash)
            }
        }
        .monacoPrivate()
    }

    private var subline: String? {
        let parts = [
            subtitle ?? row.handle.map { "@\($0)" },
            row.pricesDelayed ? "Prices delayed" : nil,
        ].compactMap { $0 }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    private static func movementSymbol(_ movement: Int) -> String {
        movement > 0 ? "arrow.up" : movement < 0 ? "arrow.down" : "minus"
    }

    private static func movementColor(_ movement: Int) -> Color {
        movement > 0 ? MonacoTheme.profit : movement < 0 ? MonacoTheme.loss : MonacoTheme.secondaryText
    }

    static func spoken(_ row: LeaderboardRowView, figure: Figure, movement: Int? = nil) -> String {
        var sentence = row.rank == 1 ? "First, \(row.name)" : "Rank \(row.rank), \(row.name)"
        if row.isViewer { sentence += ", \(viewerLabel)" }
        if let bps = row.returnBps {
            sentence += ", \(PnLSpeech.percent(PercentFormatter.format(basisPoints: bps, signed: true)))"
        } else {
            sentence += ", no return yet"
        }
        switch figure {
        case .pnl, .returnPercent:
            sentence += ", \(PnLSpeech.dollars(UsdAmountFormatter.format(signedMicros: row.pnlMicros)))"
        case .value:
            sentence += ", pot \(UsdAmountFormatter.format(micros: row.valueMicros))"
        }
        if row.pricesDelayed { sentence += ", prices delayed" }
        if let movement { sentence += ", \(spokenMovement(movement))" }
        return sentence
    }

    static func spokenMovement(_ movement: Int) -> String {
        let places = abs(movement)
        let unit = places == 1 ? "place" : "places"
        if movement > 0 { return "up \(places) \(unit)" }
        if movement < 0 { return "down \(places) \(unit)" }
        return "unchanged"
    }
}

struct BoardRowSkeleton: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var rows: Int = 3

    var body: some View {
        VStack(spacing: 0) {
            ForEach(0..<rows, id: \.self) { index in
                HStack(spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(
                        width: BoardRowMetrics.avatarSize, height: BoardRowMetrics.avatarSize,
                        radius: BoardRowMetrics.avatarSize / 2)
                    SkeletonBlock(
                        width: BoardRowMetrics.skeletonNameWidth, height: BoardRowMetrics.skeletonBarHeight,
                        radius: SkeletonMetrics.lineRadius)
                    Spacer(minLength: MonacoTheme.Space.s)
                    SkeletonBlock(
                        width: BoardRowMetrics.skeletonFigureWidth, height: BoardRowMetrics.skeletonBarHeight,
                        radius: SkeletonMetrics.lineRadius)
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.vertical, MonacoTheme.Space.sm)
                .frame(minHeight: MonacoRowLayout.minHeight)
                .overlay(alignment: .bottom) {
                    if index < rows - 1 {
                        MonacoRule().padding(
                            .leading,
                            MonacoRowLayout(dynamicTypeSize: dynamicTypeSize)
                                .separatorLeadingInset(markSize: BoardRowMetrics.avatarSize))
                    }
                }
            }
        }
        .accessibilityHidden(true)
    }
}
