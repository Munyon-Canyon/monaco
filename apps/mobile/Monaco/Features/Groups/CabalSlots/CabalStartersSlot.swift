import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalStartersSlot: CabalSection {
    static let isLive = true

    static func body(for _: CabalContext) -> some View {
        CabalEmptyReader { isEmpty in
            if isEmpty {
                CabalStartersList()
            } else {
                Color.clear.frame(height: 0)
            }
        }
    }
}

private struct CabalStartersList: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.cabalModel) private var cabalModel
    @State private var model: CabalStartersModel?

    var body: some View {
        content
            .task {
                let model = self.model ?? CabalStartersModel(api: environment.api, hints: environment.hints)
                self.model = model
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private var isReadOnly: Bool {
        cabalModel?.cabal.map { CabalClosedModel.isReadOnly($0.status) } ?? false
    }

    @ViewBuilder private var content: some View {
        if !isReadOnly, let rows = model?.rows, !rows.isEmpty {
            VStack(alignment: .leading, spacing: 0) {
                MonacoSectionHeader(CabalStartersModel.title)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                ForEach(rows) { row in
                    Button {
                        environment.navigator.open(
                            AssetRoute(symbol: row.asset.symbol), in: hostMainTab ?? environment.navigator.selectedTab)
                    } label: {
                        CabalStarterRow(starter: row, isLast: row.id == rows.last?.id)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-starter-\(row.asset.symbol)")
                }
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabal-starters")
        } else {
            Color.clear.frame(height: 0)
        }
    }
}

private struct CabalStarterRow: View {
    let starter: CabalStarter
    let isLast: Bool

    var body: some View {
        let asset = starter.asset
        MonacoRow(
            title: AssetDisplayName.format(catalogName: asset.name, kind: asset.kind), subtitle: starter.subline,
            chevron: true, isLast: isLast
        ) {
            StockMark(
                symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: .row,
                logoURL: asset.logoURL)
        } trailing: {
            if let micros = asset.priceMicros {
                MoneyText(micros: micros, style: .row)
            }
            if starter.isEligible {
                Text(CabalStartersModel.eligible)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            } else if let basisPoints = asset.changeBasisPoints {
                PercentText(basisPoints: basisPoints, style: .caption)
            }
        }
    }
}
