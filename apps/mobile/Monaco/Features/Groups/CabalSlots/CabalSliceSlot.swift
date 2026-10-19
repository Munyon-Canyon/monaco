import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalSliceSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalPotModelHost(cabalID: context.cabalID) { model in
            CabalSliceBand(model: model)
        }
    }
}

struct CabalSliceBand: View {
    let model: CabalPotModel?

    @Environment(\.cabalModel) private var cabalModel

    var body: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            if let cabal = cabalModel?.cabal, cabal.me == nil {
                Color.clear.frame(height: 0)
            } else {
                band {
                    stat("Your share", id: "cabal-slice-loading") { SkeletonBlock(width: 110, height: 28) }
                    stat("Cash", id: "cabal-slice-cash-loading") { SkeletonBlock(width: 90, height: 28) }
                }
            }
        case .loaded(let summary):
            if let slice = summary.slice {
                band {
                    stat("Your share", id: slice == .none ? "cabal-slice-none" : "cabal-slice-share") {
                        figure(slice.value, id: "cabal-slice-value")
                    }
                    stat("Cash", id: "cabal-slice-cash-cell") { figure(summary.cash, id: "cabal-slice-cash") }
                }
            } else {
                Color.clear.frame(height: 0)
            }
        case .failed:
            Color.clear.frame(height: 0)
        }
    }

    private func band(@ViewBuilder _ content: () -> some View) -> some View {
        VStack(spacing: 0) {
            MonacoRule()
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .top, spacing: MonacoTheme.Space.gutter) { content() }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) { content() }
            }
            .padding(.vertical, MonacoTheme.Space.m)
            .frame(maxWidth: .infinity, alignment: .leading)
            MonacoRule()
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private func stat(_ title: String, id: String, @ViewBuilder _ figure: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(title)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
            figure()
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(id)
    }

    private func figure(_ text: String, id: String) -> some View {
        Text(text)
            .moneyFont(.card)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(1)
            .minimumScaleFactor(MoneyStyle.card.minimumScaleFactor)
            .accessibilityIdentifier(id)
    }
}

extension CabalPotSummary.Slice {
    var value: String {
        switch self {
        case .none: "$0.00"
        case .stake(let value, _, _): value
        }
    }
}
