import MonacoCore
import SwiftUI

/// One line of a leaderboard: the rank in the market's voice, the face, the name, and the
/// return on the right. Rank 1 wears the crown.
///
/// Every board in the app — Home's investors, the cabal's members, the platform's cabals — is
/// the same argument ("who is ahead"), so it is one row. The crown is the one place gold draws
/// as a glyph rather than as a coin: it means *first*, and the row prints the number beside it
/// so the meaning never rests on the colour alone.
private let boardRankColumnWidth: CGFloat = 24

struct BoardRow<Leading: View>: View {
    /// The figure under the return: a signed P&L for a person, the pot value for a cabal.
    enum Figure {
        case pnl
        case value
    }

    static var viewerLabel: String { "You" }

    let row: LeaderboardRowView
    var figure: Figure = .pnl
    var isLast = false
    var chevron = false
    @ViewBuilder let leading: Leading

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .body) private var titleWidthFloor = MonacoRowLayout.baseMinimumTitleWidth

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
                        leading.frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                        labels
                        if chevron { MonacoRowChevron() }
                    }
                    figures(alignment: .leading)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else {
                HStack(spacing: MonacoTheme.Space.sm) {
                    rankColumn
                    leading.frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
                    labels
                        .frame(minWidth: layout.minimumTitleWidth, alignment: .leading)
                    figures(alignment: .trailing)
                        .layoutPriority(1)
                    if chevron { MonacoRowChevron() }
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, 8)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .background(row.isViewer ? MonacoTheme.brandWash : Color.clear)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.leading, ruleInset)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(Self.spoken(row, figure: figure))
    }

    private var ruleInset: CGFloat {
        let inset = layout.separatorLeadingInset(markSize: MonacoRowLayout.baseMarkSize)
        return layout.isStacked ? inset : inset + boardRankColumnWidth + MonacoTheme.Space.sm
    }

    private func figures(alignment: HorizontalAlignment) -> some View {
        VStack(alignment: alignment, spacing: 2) {
            if let bps = row.returnBps {
                PercentText(basisPoints: bps, style: .row)
            } else {
                PercentText(percentReturn: nil, style: .row)
            }
            switch figure {
            case .pnl:
                PnLText(signedMicros: row.pnlMicros, style: .caption)
            case .value:
                MoneyText(micros: row.valueMicros, style: .caption, color: MonacoTheme.muted)
            }
        }
    }

    @ViewBuilder
    private var rankColumn: some View {
        ZStack {
            if isLeader {
                Image(systemName: "crown.fill")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(MonacoTheme.goldGlyph)
            } else {
                Text("\(row.rank)")
                    .font(MonacoTheme.Typo.data)
                    .foregroundStyle(MonacoTheme.tertiaryText)
            }
        }
        .frame(width: boardRankColumnWidth, alignment: .center)
        .accessibilityHidden(true)
    }

    private var labels: some View {
        MonacoRowLabels(
            title: row.name, subtitle: row.pricesDelayed ? "Prices delayed" : nil, layout: layout
        ) {
            if row.isViewer {
                Text(Self.viewerLabel)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.brandOnWash)
            }
        }
    }

    static func spoken(_ row: LeaderboardRowView, figure: Figure) -> String {
        var sentence = row.rank == 1 ? "First, \(row.name)" : "Rank \(row.rank), \(row.name)"
        if row.isViewer { sentence += ", \(viewerLabel)" }
        if let bps = row.returnBps {
            sentence += ", \(PnLSpeech.percent(PercentFormatter.format(basisPoints: bps, signed: true)))"
        } else {
            sentence += ", no return yet"
        }
        switch figure {
        case .pnl:
            sentence += ", \(PnLSpeech.dollars(UsdAmountFormatter.format(signedMicros: row.pnlMicros)))"
        case .value:
            sentence += ", pot \(UsdAmountFormatter.format(micros: row.valueMicros))"
        }
        if row.pricesDelayed { sentence += ", prices delayed" }
        return sentence
    }
}

/// Placeholder rows in the shape of `BoardRow`: a rank slot, a 40pt face, a name and a figure,
/// ruled top and bottom like the list they stand in for.
struct BoardRowSkeleton: View {
    var rows: Int = 3

    var body: some View {
        VStack(spacing: 0) {
            ForEach(0..<rows, id: \.self) { index in
                HStack(spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(width: 18, height: 12)
                    SkeletonBlock(width: 40, height: 40, radius: 20)
                    VStack(alignment: .leading, spacing: 6) {
                        SkeletonBlock(width: 132, height: 14)
                        SkeletonBlock(width: 72, height: 11)
                    }
                    Spacer(minLength: MonacoTheme.Space.s)
                    VStack(alignment: .trailing, spacing: 6) {
                        SkeletonBlock(width: 56, height: 14)
                        SkeletonBlock(width: 40, height: 11)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.m)
                .padding(.vertical, MonacoTheme.Space.sm)
                .overlay(alignment: .bottom) {
                    if index < rows - 1 {
                        MonacoRule().padding(.leading, MonacoTheme.Space.m)
                    }
                }
            }
        }
        .overlay(alignment: .top) { MonacoRule() }
        .overlay(alignment: .bottom) { MonacoRule() }
        .accessibilityHidden(true)
    }
}
