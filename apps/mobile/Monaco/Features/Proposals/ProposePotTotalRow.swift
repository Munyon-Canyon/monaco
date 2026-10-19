import MonacoCore
import SwiftUI

struct ProposeContextCard: View {
    let cabalID: String
    let detail: ProposeContextDetail

    @Environment(AppEnvironment.self) private var environment
    @State private var context = MonacoCore.ProposeCabalContext()

    var body: some View {
        MonacoCard(style: .tight) {
            HStack(spacing: MonacoTheme.Space.sm) {
                CabalMark(
                    groupId: cabalID, name: context.name ?? "", size: ProposeContextCard.markSize,
                    pictureUrl: context.pictureURL)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(context.name ?? " ")
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(MonacoTheme.ink)
                    detailLine
                }
                Spacer(minLength: 0)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("propose-amount-pot-total")
        .task { await context.load(api: environment.api, cabalID: cabalID) }
    }

    static let markSize: CGFloat = 34

    @ViewBuilder private var detailLine: some View {
        switch detail {
        case .units(let text):
            caption(text)
        case .cash(let micros, _):
            if let micros {
                caption("\(UsdAmountFormatter.format(micros: micros)) cash")
            } else {
                SkeletonBlock(width: 90, height: 14)
            }
        case .loading:
            SkeletonBlock(width: 90, height: 14)
        }
    }

    private func caption(_ text: String) -> some View {
        Text(text).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
    }
}

enum ProposeContextDetail {
    case units(String)
    case cash(micros: Int64?, failed: Bool)
    case loading
}
