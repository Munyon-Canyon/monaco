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
                        title: "Buy a stock", detail: "Your cabal votes on it first", systemImage: ProposeChooserRow.buy
                    )
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
                title: "Sell something the cabal owns", detail: "Loading holdings", systemImage: ProposeChooserRow.sell,
                isEnabled: false, isLast: true
            )
            .redacted(reason: .placeholder)
            .accessibilityIdentifier("propose-kind-sell-loading")
        case .failed:
            MonacoErrorRow(thing: "holdings", identifier: "propose-kind-sell-failed") { Task { await pot?.load() } }
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
            systemImage: ProposeChooserRow.sell, isEnabled: !summary.sellable.isEmpty, isLast: true)
    }
}

struct ProposeChooserRow: View {
    static let buy = "arrow.down"
    static let sell = "arrow.up"

    let title: String
    let detail: String
    let systemImage: String
    var isEnabled = true
    var isLast = false

    var body: some View {
        MonacoRow(
            title: title, titleColor: isEnabled ? MonacoTheme.ink : MonacoTheme.disabledLabel, subtitle: detail,
            chevron: isEnabled, isLast: isLast
        ) {
            SunkenGlyphMark(systemImage: systemImage, isMuted: !isEnabled)
        }
    }
}
