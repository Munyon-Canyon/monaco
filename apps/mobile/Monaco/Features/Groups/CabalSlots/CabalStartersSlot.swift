import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalStartersSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalStarters()
    }
}

struct CabalStarters: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.cabalPotModel) private var potModel
    @Environment(\.cabalEmptyModel) private var emptyModel
    @State private var model: CabalStartersModel?

    private var isOn: Bool { emptyModel?.isOn(cabal: cabalModel?.cabal, pot: potModel?.summary) == true }

    var body: some View {
        content
            .task(id: isOn) {
                guard isOn else { return }
                let model = preparedModel()
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
    }

    @ViewBuilder private var content: some View {
        if isOn, let model {
            switch model.state {
            case .idle, .loading:
                VStack(alignment: .leading, spacing: 0) {
                    MonacoSectionHeader(CabalStartersModel.title)
                    MonacoRowSkeleton(rows: 3)
                }
                .accessibilityHidden(true)
            case .failed:
                Color.clear.frame(height: 0)
            case .loaded(let starters):
                if starters.isEmpty {
                    Color.clear.frame(height: 0)
                } else {
                    list(starters)
                }
            }
        } else {
            Color.clear.frame(height: 0)
        }
    }

    private func list(_ starters: [CabalStarter]) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(CabalStartersModel.title)
            ForEach(Array(starters.enumerated()), id: \.element.id) { index, starter in
                Button {
                    environment.navigator.open(
                        AssetRoute(symbol: starter.asset.symbol, seed: AssetSeed(starter.asset)),
                        in: hostMainTab ?? environment.navigator.selectedTab)
                } label: {
                    CabalStarterRow(starter: starter, isLast: index == starters.count - 1)
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier("cabal-starter-\(starter.asset.symbol)")
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-starters")
    }

    private func preparedModel() -> CabalStartersModel {
        if let model { return model }
        let created = CabalStartersModel(api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

struct CabalStarterRow: View {
    let starter: CabalStarter
    var isLast = false

    var body: some View {
        MonacoRow(
            title: AssetDisplayName.format(catalogName: starter.asset.name, kind: starter.asset.kind),
            subtitle: starter.subline,
            chevron: true,
            isLast: isLast
        ) {
            StockMark(
                symbol: starter.asset.symbol, displayName: starter.asset.name, assetKind: starter.asset.kind,
                size: 40, logoURL: starter.asset.logoURL)
        } trailing: {
            if let micros = starter.priceMicros {
                MoneyText(micros: micros, style: .row)
            }
            if let eligibility = starter.eligibility {
                Text(eligibility)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            } else if let basisPoints = starter.changeBasisPoints {
                PercentText(basisPoints: basisPoints, style: .caption)
            }
        }
    }
}
