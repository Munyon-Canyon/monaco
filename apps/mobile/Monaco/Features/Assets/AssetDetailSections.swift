import MonacoCore
import SwiftUI

struct AssetMarketStats: View {
    let stats: AssetStats?
    let attribution: String
    let priceMicros: Int64?
    let volume24hMicros: Int64?

    private var items: [StatItem] {
        let cells = stats?.cells.map { StatItem(label: $0.label, value: $0.value) } ?? []
        guard let volume24hMicros else { return cells }
        return cells + [StatItem(label: "24h volume", value: UsdAmountFormatter.compact(micros: volume24hMicros))]
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Market stats")
            Text(attribution)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.bottom, MonacoTheme.Space.s)
            if !items.isEmpty {
                StatGrid(items: items)
            }
            Text("Market cap and supply open soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.top, MonacoTheme.Space.sm)
            if let stats, let low = stats.yearLow, let high = stats.yearHigh,
                let basisPoints = stats.yearRangeBasisPoints(priceMicros: priceMicros)
            {
                YearRangeBar(low: low, high: high, basisPoints: basisPoints)
                    .padding(.top, MonacoTheme.Space.m)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-stats")
    }
}

struct YearRangeBar: View {
    let low: Int64
    let high: Int64
    let basisPoints: Int64

    private static let trackHeight: CGFloat = 7

    var body: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            GeometryReader { proxy in
                ZStack(alignment: .leading) {
                    Capsule().fill(MonacoTheme.border)
                    Capsule()
                        .fill(MonacoTheme.brand)
                        .frame(width: max(Self.trackHeight, proxy.size.width * CGFloat(basisPoints) / 10_000))
                }
            }
            .frame(height: Self.trackHeight)
            HStack {
                Text(UsdAmountFormatter.format(micros: low))
                Spacer()
                Text(UsdAmountFormatter.format(micros: high))
            }
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            "52-week range, \(UsdAmountFormatter.format(micros: low)) to \(UsdAmountFormatter.format(micros: high))"
        )
        .accessibilityIdentifier("asset-year-range")
    }
}

struct AssetAbout: View {
    let detail: AssetDetailPresentation

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(detail.name)
            if detail.kind == .preIpo {
                Text(PreIpoCopy.disclosure)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.bodyText)
                    .lineSpacing(MonacoTheme.Space.xs)
            } else {
                Text("Company profile opens soon.")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-about")
    }
}

struct AssetCabalHoldings: View {
    let ticker: String
    let positions: [AssetCabalPosition]
    let open: (AssetCabalPosition) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Cabals holding \(ticker)").padding(.horizontal, MonacoTheme.Space.gutter)
            Text("Your cabals")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.s)
            if positions.isEmpty {
                EmptyState(
                    title: "No visible holders", message: "Private holdings are not shown.", systemImage: "person.3")
            } else {
                ForEach(positions) { position in
                    Button {
                        open(position)
                    } label: {
                        row(position)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("asset-position-\(position.cabalID)")
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-cabal-positions")
    }

    private func row(_ position: AssetCabalPosition) -> some View {
        MonacoRow(
            title: position.cabalName, subtitle: position.sharesLabel, chevron: true,
            isLast: position.id == positions.last?.id
        ) {
            CabalCover(
                cabalId: position.cabalID, tint: .forGroupId(position.cabalID),
                pictureURL: CabalMark.resolvedURL(position.pictureURL), style: .thumb(40)
            )
            .frame(width: 40, height: 40)
        } trailing: {
            MoneyText(micros: position.valueMicros, style: .row)
            if let basisPoints = position.returnBasisPoints {
                PnLBadge(signedMicros: position.pnlMicros, basisPoints: basisPoints)
            }
        }
    }
}

struct AssetProposals: View {
    let ticker: String

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Proposals for \(ticker)")
            #if DEBUG
            if let sample = Self.sampleCard {
                sample
            } else {
                quiet
            }
            #else
            quiet
            #endif
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-proposals")
    }

    private var quiet: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoQuietEmpty("No visible proposals for this asset.", systemImage: "checkmark.circle")
            Text("Proposals by stock open soon.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    #if DEBUG
    @MainActor static var sampleCard: AnyView?
    #endif
}

struct AssetOtherListings: View {
    let listings: [AssetListingPresentation]
    let open: (AssetListingPresentation) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Also available from").padding(.horizontal, MonacoTheme.Space.gutter)
            ForEach(listings) { listing in
                Button {
                    open(listing)
                } label: {
                    MonacoRow(
                        title: listing.name, subtitle: listing.ticker, chevron: true,
                        isLast: listing.id == listings.last?.id
                    ) {
                        StockMark(symbol: listing.symbol, displayName: listing.name, assetKind: listing.kind)
                    } trailing: {
                        if let issuer = listing.issuer.displayName {
                            Text(issuer).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.secondaryText)
                        }
                    }
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("asset-other-listing-\(listing.symbol)")
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-other-listings")
    }
}
