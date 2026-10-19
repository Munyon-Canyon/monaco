import MonacoAPI
import MonacoCore
import SwiftUI

struct AssetDetailHeader: View {
    let detail: AssetDetailPresentation
    let model: AssetDetailClientModel
    let scrub: AssetScrubHeader?
    let priceTick: MonacoPriceTick?
    let bestChain: AssetBestChain?
    let isWatched: Bool
    let toggleWatch: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            identity
                .padding(.top, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.m)
            notices
            price
            change.padding(.top, MonacoTheme.Space.s)
            session
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var identity: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: detail.symbol, displayName: detail.name, assetKind: detail.kind, size: 48,
                logoURL: detail.market.logoURL)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(detail.name)
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .accessibilityIdentifier("asset-detail-name")
                Text(detail.ticker)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            Spacer(minLength: 0)
            WatchStarButton(
                isWatched: isWatched, addLabel: "Add to watchlist", removeLabel: "Remove from watchlist",
                identifier: "asset-watch-star", toggle: toggleWatch)
        }
    }

    @ViewBuilder private var notices: some View {
        let shown = model.notices
        if !shown.isEmpty {
            VStack(spacing: MonacoTheme.Space.s) {
                ForEach(Array(shown.enumerated()), id: \.offset) { _, notice in
                    MonacoNotice(.warn, message: notice.text)
                }
            }
            .padding(.bottom, MonacoTheme.Space.m)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("asset-detail-notices")
        }
    }

    @ViewBuilder private var price: some View {
        if let scrub {
            Text(scrub.price)
                .moneyFont(.hero)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .accessibilityIdentifier("asset-detail-price")
        } else if let micros = detail.priceMicros {
            Text(UsdAmountFormatter.format(micros: micros))
                .moneyFont(.hero)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .priceTickFlash(
                    priceTick, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous),
                    expand: MonacoTheme.Space.xs
                )
                .accessibilityIdentifier("asset-detail-price")
        } else {
            Text("—")
                .moneyFont(.hero)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
        }
    }

    @ViewBuilder private var change: some View {
        if let scrub {
            changeLine(basisPoints: scrub.basisPoints, caption: scrub.caption)
        } else if let change = model.rangeChange {
            changeLine(basisPoints: change.basisPoints, caption: change.label)
                .opacity(change.basisPoints == nil ? 0 : 1)
                .accessibilityHidden(change.basisPoints == nil)
        }
    }

    private func changeLine(basisPoints: Int64?, caption: String) -> some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: MonacoTheme.Space.sm) { changeParts(basisPoints, caption) }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) { changeParts(basisPoints, caption) }
        }
        .accessibilityIdentifier("asset-detail-range-change")
    }

    @ViewBuilder
    private func changeParts(_ basisPoints: Int64?, _ caption: String) -> some View {
        if let basisPoints { PercentText(basisPoints: basisPoints, style: .row) }
        Text(caption).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
    }

    @ViewBuilder private var session: some View {
        let copy = detail.market.showsSessionChip ? MarketSessionCopy.chip(for: detail.market.status) : nil
        if copy != nil || bestChain != nil {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                if let copy { MarketSessionChip(session: copy).accessibilityIdentifier("asset-detail-session") }
                if let bestChain { AssetBestChainLine(bestChain: bestChain) }
            }
            .padding(.top, MonacoTheme.Space.s)
        }
    }
}

struct AssetBestChainLine: View {
    let bestChain: AssetBestChain

    var body: some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            ChainMark(network: bestChain.network.rawValue, size: 24)
            Text(bestChain.headline)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("asset-detail-best-chain")
    }
}
