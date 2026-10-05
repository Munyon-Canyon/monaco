import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct ProposeRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        ProposeChooserScreen(cabalID: cabalID)
    }
}

struct ProposeChooserScreen: View {
    let cabalID: String

    var body: some View {
        ScrollView {
            MonacoGroupedList {
                NavigationLink {
                    ProposeBuyStockView(cabalID: cabalID)
                } label: {
                    ProposeChooserRow(
                        title: "Buy a stock", detail: "Your cabal votes on it first", systemImage: ProposeGlyph.buy)
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("propose-kind-buy")

                CabalPotModelHost(cabalID: cabalID) { pot in
                    ProposeSellChooserRow(pot: pot)
                }
            }
            .padding(.top, MonacoTheme.Space.s)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .navigationTitle("Propose")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("propose-chooser")
    }
}

private struct ProposeSellChooserRow: View {
    let pot: CabalPotModel?

    var body: some View {
        switch pot?.state ?? .loading {
        case .idle, .loading:
            ProposeChooserRow(
                title: "Sell something the cabal owns", detail: "Loading holdings", systemImage: ProposeGlyph.sell,
                isEnabled: false, isLast: true
            )
            .redacted(reason: .placeholder)
            .accessibilityIdentifier("propose-kind-sell-loading")
        case .failed:
            Button {
                Task { await pot?.load() }
            } label: {
                ProposeChooserRow(
                    title: "Sell something the cabal owns", detail: "Couldn't load holdings.",
                    systemImage: ProposeGlyph.sell, retryTitle: "Try again", isLast: true)
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("propose-kind-sell-failed")
        case .loaded(let summary):
            if let pot, !summary.sellable.isEmpty {
                NavigationLink {
                    ProposeSellView(pot: pot)
                } label: {
                    row(summary)
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("propose-kind-sell")
            } else {
                row(summary).accessibilityIdentifier("propose-kind-sell")
            }
        }
    }

    private func row(_ summary: CabalPotSummary) -> some View {
        ProposeChooserRow(
            title: "Sell something the cabal owns", detail: ProposeHolding.chooserDetail(summary.sellable),
            systemImage: ProposeGlyph.sell, isEnabled: !summary.sellable.isEmpty, isLast: true)
    }
}

struct ProposeChooserRow: View {
    let title: String
    let detail: String
    let systemImage: String
    var isEnabled = true
    var retryTitle: String?
    var isLast = false

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            ProposeGlyph(systemImage: systemImage, isEnabled: isEnabled)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(isEnabled ? MonacoTheme.ink : MonacoTheme.disabledLabel)
                    .fixedSize(horizontal: false, vertical: true)
                Text(detail)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            if let retryTitle {
                Text(retryTitle).font(MonacoTheme.Typo.bodyStrong).foregroundStyle(MonacoTheme.ink)
            } else if isEnabled {
                Image(systemName: "chevron.right")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(MonacoTheme.tertiaryText)
                    .accessibilityHidden(true)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, 10)
        .frame(minHeight: 64)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.leading, MonacoTheme.Space.m + ProposeGlyph.rowSize + MonacoTheme.Space.sm)
            }
        }
        .accessibilityElement(children: .combine)
    }
}

struct ProposeGlyph: View {
    let systemImage: String
    var isEnabled = true

    static let rowSize: CGFloat = 40
    static let buy = "arrow.down"
    static let sell = "arrow.up"

    var body: some View {
        Image(systemName: systemImage)
            .font(.system(size: (Self.rowSize * 0.4).rounded(), weight: .semibold))
            .foregroundStyle(isEnabled ? MonacoTheme.ink : MonacoTheme.disabledLabel)
            .frame(width: Self.rowSize, height: Self.rowSize)
            .background(Circle().fill(MonacoTheme.surfaceSunken))
            .accessibilityHidden(true)
    }
}
