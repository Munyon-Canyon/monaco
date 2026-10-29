import MonacoCore
import SwiftUI

enum PreIpoPageMetrics {
    static let heroHeight: CGFloat = 120
    static let markSize: CGFloat = 48
    static let loneMarkSize: CGFloat = 56
    static let heroMaxPixelSize = 1_200
    static let heroCacheLimit = 12
    static let ledgerRowHeight: CGFloat = 52
}

enum PreIpoPageState {
    case loading
    case failed
    case loaded(AssetDetailPresentation)
}

struct PreIpoDetailView: View {
    let symbol: String
    let name: String
    let logoURL: URL?
    let state: PreIpoPageState
    let held: [AssetHeldRowFacts]
    let proposals: [AssetOpenProposalRowFacts]
    let openCabal: (String) -> Void
    let openProposal: (String) -> Void
    let propose: (ProposeKind) -> Void
    let retry: () -> Void

    @State private var range: PreIpoChartRange = .all
    @State private var infoSheet: Info?
    @State private var nameVisible = true

    private enum Info: Identifiable {
        case estimate
        case roundStats

        var id: Self { self }
    }

    private var detail: AssetDetailPresentation? {
        if case .loaded(let detail) = state { detail } else { nil }
    }

    private var preIpo: PreIpoDetail? { detail?.preIpo }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.section) {
                header
                body(for: state)
                if let detail {
                    AssetDetailActions(
                        detail: detail, canSell: !held.isEmpty,
                        showsBuy: preIpo?.eligibility.allowsBuy ?? true, propose: propose)
                }
                AssetInYourCabals(
                    held: held, proposals: proposals, openCabal: openCabal, openProposal: openProposal)
                if let profile = preIpo?.profile {
                    PreIpoRoundStats(top: profile.statsTop, bottom: profile.statsBottom) {
                        infoSheet = .roundStats
                    }
                }
            }
            .padding(.top, MonacoTheme.Space.xs)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .monacoCanvas()
        .foregroundStyle(MonacoTheme.primaryText)
        .monacoFrameStats("AssetDetail")
        .toolbar {
            if !nameVisible {
                ToolbarItem(placement: .principal) {
                    Text(name)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .lineLimit(1)
                        .accessibilityIdentifier("asset-pre-ipo-bar-title")
                }
            }
        }
        .sheet(item: $infoSheet) { info in
            switch info {
            case .estimate:
                PreIpoInfoSheet(
                    title: PreIpoCopy.estimateTitle, text: PreIpoCopy.estimateInfo,
                    identifier: "asset-pre-ipo-estimate-info")
            case .roundStats:
                PreIpoInfoSheet(
                    title: PreIpoCopy.roundStatsTitle, text: PreIpoCopy.roundStatsInfo,
                    identifier: "asset-pre-ipo-stats-info")
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-detail-root")
    }

    @ViewBuilder
    private func body(for state: PreIpoPageState) -> some View {
        switch state {
        case .loading:
            placeholders
        case .failed:
            unavailable(PreIpoCopy.loadFailed)
        case .loaded:
            if let profile = preIpo?.profile {
                estimateBlock(profile)
                ledger(profile)
                eligibility
                chart(profile)
            } else {
                eligibility
                unavailable(PreIpoCopy.estimateUnavailable)
            }
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            hero
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(name)
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
                    .onScrollVisibilityChange(threshold: 0.01) { nameVisible = $0 }
                    .accessibilityAddTraits(.isHeader)
                    .accessibilityIdentifier("asset-detail-name")
                Text(PreIpoCopy.privateLine)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("asset-pre-ipo-private-line")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }

    @ViewBuilder
    private var hero: some View {
        if let url = preIpo?.profile?.imageURL {
            PreIpoHeroImage(url: url) { mark(PreIpoPageMetrics.markSize) }
                .padding(.horizontal, MonacoTheme.Space.gutter)
        } else {
            mark(PreIpoPageMetrics.loneMarkSize)
                .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }

    private func mark(_ size: CGFloat) -> some View {
        StockMark(symbol: symbol, displayName: name, assetKind: .preIpo, size: size, logoURL: logoURL)
            .accessibilityHidden(true)
    }
}

extension PreIpoDetailView {
    private func estimateBlock(_ profile: PreIpoDetail.Profile) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 0) {
                Text(PreIpoCopy.estimateTitle)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                Button {
                    infoSheet = .estimate
                } label: {
                    Image(systemName: "info.circle")
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.tint)
                        .frame(width: StatMetrics.infoTarget, height: StatMetrics.infoTarget)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(PreIpoCopy.estimateInfoLabel)
                .accessibilityIdentifier("asset-pre-ipo-estimate-info-button")
                Spacer(minLength: 0)
            }
            .padding(.bottom, -MonacoTheme.Space.s)
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .lastTextBaseline, spacing: MonacoTheme.Space.s) {
                    price(profile)
                    caption(profile)
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    price(profile)
                    caption(profile)
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-pre-ipo-estimate")
    }

    private func price(_ profile: PreIpoDetail.Profile) -> some View {
        let parts = AssetDetailHeader.heroParts(profile.estimateText)
        return (Text(verbatim: parts.whole) + Text(verbatim: parts.cents).foregroundStyle(MonacoTheme.secondaryText))
            .moneyFigure(.hero)
            .lineLimit(1)
            .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
            .accessibilityLabel("\(PreIpoCopy.estimateTitle), \(profile.estimateText), \(profile.estimateCaption)")
            .accessibilityIdentifier("asset-pre-ipo-price")
    }

    private func caption(_ profile: PreIpoDetail.Profile) -> some View {
        Text(profile.estimateCaption)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .accessibilityHidden(true)
    }

    private func ledger(_ profile: PreIpoDetail.Profile) -> some View {
        VStack(spacing: 0) {
            ForEach(Array(profile.ledger.enumerated()), id: \.offset) { index, line in
                if index > 0 { MonacoRule() }
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                    Text(line.label)
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Spacer(minLength: MonacoTheme.Space.s)
                    Text(line.value)
                        .moneyFigure(.row, weight: .regular)
                        .foregroundStyle(MonacoTheme.primaryText)
                }
                .frame(minHeight: PreIpoPageMetrics.ledgerRowHeight)
                .padding(.horizontal, StatMetrics.cardPaddingHorizontal)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("\(line.label), \(line.value)")
            }
        }
        .background(
            MonacoTheme.surface, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
        )
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-pre-ipo-ledger")
    }

    @ViewBuilder
    private var eligibility: some View {
        if let preIpo {
            let eligible = preIpo.eligibility.allowsBuy
            HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.xs) {
                Image(systemName: eligible ? "checkmark.circle.fill" : "minus.circle.fill")
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(eligible ? MonacoTheme.profit : MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
                (Text(preIpo.eligibility.title).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
                    + Text(" · ").font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
                    + Text(preIpo.eligibility.detail).font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText))
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("\(preIpo.eligibility.title), \(preIpo.eligibility.detail)")
            .accessibilityIdentifier("asset-pre-ipo-eligibility")
        }
    }

    private func chart(_ profile: PreIpoDetail.Profile) -> some View {
        let steps = profile.steps(in: range, now: WallClock.now())
        return VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            PreIpoRoundsChart(steps: steps)
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoTheme.Space.s) {
                    countLine(profile)
                    ranges
                }
                VStack(alignment: .leading, spacing: 0) {
                    countLine(profile)
                    ranges
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }

    private func countLine(_ profile: PreIpoDetail.Profile) -> some View {
        Text(profile.countLine)
            .font(MonacoTheme.Typo.subheadStrong)
            .foregroundStyle(MonacoTheme.secondaryText)
            .lineLimit(1)
            .fixedSize()
            .accessibilityIdentifier("asset-pre-ipo-count")
    }

    private var ranges: some View {
        MonacoRangeChips(
            ranges: PreIpoChartRange.allCases, selection: range, identifierPrefix: "asset-pre-ipo-range",
            label: { $0.label }, onSelect: { range = $0 }
        )
        .accessibilityIdentifier("asset-pre-ipo-ranges")
    }

    private var placeholders: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            SkeletonFraction(SkeletonMetrics.rowSubtitleFraction) { SkeletonBar() }
            SkeletonBlock(height: PreIpoPageMetrics.ledgerRowHeight, radius: MonacoTheme.Radius.card)
            SkeletonBlock(height: PreIpoPageMetrics.ledgerRowHeight, radius: MonacoTheme.Radius.card)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading estimate")
        .accessibilityIdentifier("asset-pre-ipo-loading")
    }

    private func unavailable(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            Text(message)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityIdentifier("asset-pre-ipo-unavailable")
            Button(PreIpoCopy.retry, action: retry)
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("asset-pre-ipo-retry")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}

private struct PreIpoHeroImage<Mark: View>: View {
    let url: URL
    let mark: () -> Mark

    @State private var image: UIImage?

    private static var store: MonacoRemoteImageStore { PreIpoHeroStore.shared }

    var body: some View {
        RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
            .fill(MonacoTheme.surface)
            .frame(height: PreIpoPageMetrics.heroHeight)
            .frame(maxWidth: .infinity)
            .overlay {
                if let image = image ?? Self.store.cachedImage(for: url) {
                    Image(uiImage: image)
                        .resizable()
                        .scaledToFill()
                        .accessibilityHidden(true)
                }
            }
            .overlay(alignment: .bottomLeading) {
                mark().padding(MonacoTheme.Space.sm)
            }
            .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
            .task(id: url) { image = await Self.store.image(for: url) }
            .accessibilityIdentifier("asset-pre-ipo-hero")
    }
}

@MainActor
private enum PreIpoHeroStore {
    static let shared = MonacoRemoteImageStore(
        maxPixelSize: PreIpoPageMetrics.heroMaxPixelSize, countLimit: PreIpoPageMetrics.heroCacheLimit)
}
