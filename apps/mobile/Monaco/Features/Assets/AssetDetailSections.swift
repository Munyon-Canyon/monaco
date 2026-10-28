import MonacoCore
import SwiftUI

struct AssetMarketStats: View {
    let stats: AssetStats
    let showInfo: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack(spacing: 0) {
                Text(AssetStatsInfoCopy.title)
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .accessibilityAddTraits(.isHeader)
                Button(action: showInfo) {
                    Image(systemName: "info.circle")
                        .font(MonacoTheme.Typo.section)
                        .foregroundStyle(MonacoTheme.tint)
                        .frame(width: StatMetrics.infoTarget, height: StatMetrics.infoTarget)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("About market stats")
                .accessibilityIdentifier("asset-stats-info-button")
                Spacer(minLength: 0)
            }
            StatGrid(items: Self.items(stats.topRow), columns: 3)
            StatGrid(items: Self.items(stats.bottomRow), columns: 3)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("asset-stats")
    }

    static func items(_ cells: [AssetStatCell]) -> [StatItem] {
        cells.map { StatItem(label: $0.label, value: $0.value, spokenValue: $0.spokenValue) }
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
