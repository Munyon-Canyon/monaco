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

                ProposeChooserRow(
                    title: "Sell something the cabal owns", detail: "Nothing to sell yet",
                    systemImage: ProposeGlyph.sell, isEnabled: false, isLast: true
                )
                .accessibilityIdentifier("propose-kind-sell")
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

struct ProposeChooserRow: View {
    let title: String
    let detail: String
    let systemImage: String
    var isEnabled = true
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
            if isEnabled {
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
