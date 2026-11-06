import MonacoAPI
import MonacoCore
import SwiftUI

enum HomeTotalPartsSlot: HomeSection {
    static let isLive = true

    static func shows(summary: PortfolioSummary?, sharedFailure: Bool) -> Bool {
        guard !sharedFailure else { return false }
        guard let summary else { return true }
        return !summary.isFirstUse
    }

    static func body(for context: Void) -> some View {
        HomeTotalParts()
    }
}

enum HomeTotalPartsMetrics {
    static let cover: CGFloat = 32
    static let overlap: CGFloat = 20
    static let maxCovers = 3
    static let skeletonWidth: CGFloat = 96
}

private struct HomeTotalParts: View {
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @Environment(\.displayScale) private var displayScale

    var body: some View {
        let hidden = !HomeTotalPartsSlot.shows(
            summary: portfolio?.summary, sharedFailure: reads?.plan.showsSharedRow == true)
        MonacoCard(style: .inset) {
            VStack(alignment: .leading, spacing: 0) {
                HomeSharesRow()
                Rectangle()
                    .fill(MonacoTheme.border)
                    .frame(height: 1 / displayScale)
                    .padding(.trailing, -MonacoTheme.Space.m)
                HomeCashRowSection(embedded: true)
                    .padding(.vertical, MonacoTheme.Space.sm)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .homeSlotHidden(hidden)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-total-parts")
    }
}

private struct HomeSharesRow: View {
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads

    var body: some View {
        switch portfolio?.state ?? .loading {
        case .idle, .loading: loading
        case .failed: failed
        case .loaded: loaded
        }
    }

    private var loading: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            SkeletonBlock(width: HomeTotalPartsMetrics.cover, height: HomeTotalPartsMetrics.cover)
                .frame(width: HomeCashRowMetrics.partColumn)
            VStack(alignment: .leading, spacing: 0) {
                Text(" ").font(MonacoTheme.Typo.headline).skeletonBar(width: HomeTotalPartsMetrics.skeletonWidth)
                Text(" ").font(MonacoTheme.Typo.subhead)
            }
            Spacer(minLength: 0)
        }
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: MonacoButtonMetrics.hitTarget)
        .accessibilityElement()
        .accessibilityLabel("Loading your share in cabals")
        .accessibilityIdentifier("home-shares-loading")
    }

    @ViewBuilder private var failed: some View {
        if reads.showsOwnRow(.portfolio) {
            MonacoErrorRow(thing: "your cabals", identifier: "home-shares-error", inset: false) {
                Task { await portfolio?.load() }
            }
            .padding(.vertical, MonacoTheme.Space.sm)
        }
    }

    @ViewBuilder private var loaded: some View {
        if let summary = portfolio?.summary {
            let count = summary.rows.count
            NavigationLink(value: AnyAppRoute(BalancesRoute())) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    covers(summary.rows)
                        .frame(width: HomeCashRowMetrics.partColumn, alignment: .leading)
                    VStack(alignment: .leading, spacing: 0) {
                        MoneyText(micros: UsdAmountFormatter.flooredToCents(summary.totalMicros), style: .row)
                            .lineLimit(1)
                            .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
                        Text(HomeFigureCopy.inCabals(count: count))
                            .font(MonacoTheme.Typo.subhead)
                            .foregroundStyle(MonacoTheme.secondaryText)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    Image(systemName: MonacoIcon.chevron.symbol)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .accessibilityHidden(true)
                }
                .padding(.vertical, MonacoTheme.Space.sm)
                .frame(minHeight: MonacoButtonMetrics.hitTarget)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(HomeFigureCopy.sharesSpoken(count: count, micros: summary.totalMicros))
            .accessibilityHint(HomeFigureCopy.sharesHint)
            .accessibilityAddTraits(.isButton)
            .accessibilityIdentifier("home-shares-row")
        }
    }

    private func covers(_ rows: [PortfolioSummary.Row]) -> some View {
        HStack(spacing: -HomeTotalPartsMetrics.overlap) {
            ForEach(rows.prefix(HomeTotalPartsMetrics.maxCovers)) { row in
                CabalMark(
                    groupId: row.id, name: row.name, size: HomeTotalPartsMetrics.cover, pictureUrl: row.pictureURL
                )
                .overlay {
                    MarkGeometry.shape(.tile, size: HomeTotalPartsMetrics.cover)
                        .stroke(MonacoTheme.surface, lineWidth: MonacoAvatarRing.width)
                }
            }
        }
    }
}
