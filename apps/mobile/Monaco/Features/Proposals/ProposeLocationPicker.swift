import MonacoAPI
import MonacoCore
import SwiftUI

struct ProposeLocationSection: View {
    let holding: MonacoCore.ProposeHolding
    let choose: (Components.Schemas.Network) -> Void
    @State private var choosing = false

    var body: some View {
        if let network = holding.network {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader(ProposeFlowCopy.sellFromTitle)
                MonacoGroupedList {
                    Button {
                        choosing = true
                    } label: {
                        MonacoRow(
                            title: MonacoCore.BestPrice.chainName(network),
                            subtitle: UsdAmountFormatter.format(micros: holding.valueMicros), chevron: true,
                            isLast: true
                        ) {
                            ChainMark(network: network.rawValue, size: MonacoRowLayout.baseMarkSize)
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityElement(children: .combine)
                    .accessibilityHint(ProposeFlowCopy.sellFromHint)
                    .accessibilityIdentifier("propose-location-row")
                }
            }
            .sheet(isPresented: $choosing) {
                ProposeLocationSheet(holding: holding, choose: choose)
            }
        }
    }
}

struct ProposeLocationSheet: View {
    struct Row: Equatable, Identifiable {
        let id: String
        let network: Components.Schemas.Network
        let title: String
        let figure: String
        let caption: String
        let isSelected: Bool
    }

    let holding: MonacoCore.ProposeHolding
    let choose: (Components.Schemas.Network) -> Void
    @Environment(\.dismiss) private var dismiss

    static func rows(_ holding: MonacoCore.ProposeHolding) -> [Row] {
        holding.locations.map { location in
            let there = holding.at(location.network)
            return Row(
                id: "propose-location-\(location.id)", network: location.network,
                title: MonacoCore.BestPrice.chainName(location.network),
                figure: UsdAmountFormatter.format(micros: location.valueMicros),
                caption: there.quantity(of: there.tokenAmount),
                isSelected: location.network == holding.network)
        }
    }

    var body: some View {
        ScrollView {
            MonacoGroupedList {
                let rows = Self.rows(holding)
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    Button {
                        choose(row.network)
                        dismiss()
                    } label: {
                        MonacoRow(
                            title: row.title, subtitle: row.caption, chevron: false, isLast: index == rows.count - 1
                        ) {
                            ChainMark(network: row.network.rawValue, size: MonacoRowLayout.baseMarkSize)
                        } trailing: {
                            HStack(spacing: MonacoTheme.Space.s) {
                                Text(row.figure)
                                    .font(MonacoTheme.Typo.dataStrong)
                                    .foregroundStyle(MonacoTheme.ink)
                                Image(systemName: "checkmark")
                                    .font(MonacoTheme.Typo.calloutStrong)
                                    .foregroundStyle(MonacoTheme.brand)
                                    .opacity(row.isSelected ? 1 : 0)
                                    .accessibilityHidden(true)
                            }
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityElement(children: .combine)
                    .accessibilityAddTraits(row.isSelected ? .isSelected : [])
                    .accessibilityIdentifier(row.id)
                }
            }
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoSheet(title: ProposeFlowCopy.sellFromTitle, detents: .votes, showsClose: true)
        .accessibilityIdentifier("propose-location-sheet")
    }
}
