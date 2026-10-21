import MonacoAPI
import MonacoCore
import SwiftUI

struct ProposeChainSection: View {
    let model: MonacoCore.ProposeReviewModel
    @State private var choosing = false

    var body: some View {
        let options = model.chainOptions
        let choices = options.filter(\.isAvailable)
        if let current = choices.first(where: { $0.chain == model.chain }) ?? choices.first {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Chain")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoGroupedList {
                    Button {
                        choosing = true
                    } label: {
                        MonacoRow(title: current.title, subtitle: current.figure, chevron: true, isLast: true) {
                            ChainMark(network: current.networkID, size: MonacoRowLayout.baseMarkSize)
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityElement(children: .combine)
                    .accessibilityHint("Chooses the chain the cabal buys on")
                    .accessibilityIdentifier("propose-chain-row")
                }
                if let problem = model.chainProblem {
                    Text(problem)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                        .accessibilityIdentifier("propose-chain-problem")
                }
            }
            .sheet(isPresented: $choosing) {
                ProposeChainSheet(options: options, selected: model.chain) { model.choose($0) }
            }
        }
    }
}

struct ProposeChainSheet: View {
    struct Row: Equatable, Identifiable {
        let id: String
        let chain: MonacoCore.ProposeChain
        let network: String
        let title: String
        let figure: String
        let caption: String
        let isSelected: Bool
        var isAvailable = true
    }

    let options: [MonacoCore.ProposeChainOption]
    let selected: MonacoCore.ProposeChain
    let choose: (MonacoCore.ProposeChain) -> Void
    @Environment(\.dismiss) private var dismiss

    static func rows(_ options: [MonacoCore.ProposeChainOption], selected: MonacoCore.ProposeChain) -> [Row] {
        options.map { option in
            Row(
                id: "propose-chain-\(option.id)", chain: option.chain, network: option.networkID,
                title: option.title, figure: option.figure,
                caption: option.isBest
                    ? "The cabal buys wherever the price is best when the trade runs"
                    : "The cabal buys only on \(option.title)",
                isSelected: option.chain == selected, isAvailable: option.isAvailable)
        }
    }

    var body: some View {
        ScrollView {
            MonacoGroupedList {
                let rows = Self.rows(options, selected: selected)
                ForEach(Array(rows.enumerated()), id: \.element.id) { index, row in
                    Button {
                        choose(row.chain)
                        dismiss()
                    } label: {
                        ProposeChainRow(row: row, isLast: index == rows.count - 1)
                    }
                    .buttonStyle(.monacoRow)
                    .disabled(!row.isAvailable)
                    .accessibilityElement(children: .combine)
                    .accessibilityAddTraits(row.isSelected ? .isSelected : [])
                    .accessibilityIdentifier(row.id)
                }
            }
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoSheet(title: "Choose a chain", detents: .votes, showsClose: true)
        .accessibilityIdentifier("propose-chain-sheet")
    }
}

private struct ProposeChainRow: View {
    let row: ProposeChainSheet.Row
    let isLast: Bool

    var body: some View {
        MonacoRow(title: row.title, subtitle: row.caption, chevron: false, isLast: isLast) {
            ChainMark(network: row.network, size: MonacoRowLayout.baseMarkSize)
        } trailing: {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(row.figure)
                    .font(MonacoTheme.Typo.dataStrong)
                    .foregroundStyle(row.isAvailable ? MonacoTheme.ink : MonacoTheme.secondaryText)
                Image(systemName: "checkmark")
                    .font(MonacoTheme.Typo.calloutStrong)
                    .foregroundStyle(MonacoTheme.brand)
                    .opacity(row.isSelected ? 1 : 0)
                    .accessibilityHidden(true)
            }
        }
        .opacity(row.isAvailable ? 1 : 0.45)
    }
}

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
