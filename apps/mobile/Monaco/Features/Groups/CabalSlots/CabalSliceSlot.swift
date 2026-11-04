import MonacoAPI
import MonacoCore
import SwiftUI

struct CabalSliceBand: View {
    let model: CabalPotModel?
    let cabalKnown: Bool
    let isMember: Bool

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    static let unknown = "\u{2013}"

    var isShown: Bool {
        switch model?.state ?? .loading {
        case .idle, .loading: !(cabalKnown && !isMember)
        case .loaded(let summary): summary.slice != nil || isMember
        case .failed: isMember
        }
    }

    var body: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            fact {
                Text("$0").moneyFigure(.row).skeletonBar(width: 72)
            }
            .accessibilityIdentifier("cabal-slice-loading")
        case .loaded(let summary):
            let share = summary.slice != nil ? CabalHeaderCopy.wholeDollars(summary.shareMicros) : "$0"
            let slice = summary.slice != nil ? CabalHeaderCopy.percent(basisPoints: summary.sliceBps) : "0%"
            loaded(share: share, slice: slice)
        case .failed:
            loaded(share: Self.unknown, slice: nil)
        }
    }

    private func loaded(share: String, slice: String?) -> some View {
        fact {
            let layout =
                dynamicTypeSize >= .xxxLarge
                ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
                : AnyLayout(HStackLayout(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s))
            layout {
                Text(share)
                    .moneyFigure(.row)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.row.minimumScaleFactor)
                if let slice {
                    Text(slice)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
            }
        }
        .monacoPrivate()
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spoken(share: share, slice: slice))
        .accessibilityIdentifier("cabal-share-value")
    }

    private func fact<Value: View>(@ViewBuilder value: () -> Value) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("Your share")
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
            value()
        }
        .frame(minHeight: MonacoButtonMetrics.hitTarget, alignment: .topLeading)
    }

    static func spoken(share: String, slice: String?) -> String {
        guard let slice else { return "Your share, \(share)" }
        let percent =
            slice.hasPrefix("<")
            ? "less than \(slice.dropFirst().dropLast()) percent" : "\(slice.dropLast()) percent"
        return "Your share, \(share), \(percent) of the treasury"
    }
}
