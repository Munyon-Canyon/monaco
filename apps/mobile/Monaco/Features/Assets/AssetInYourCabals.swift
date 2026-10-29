import MonacoCore
import SwiftUI

struct AssetInYourCabals: View {
    let held: [AssetHeldRowFacts]
    let proposals: [AssetOpenProposalRowFacts]
    let openCabal: (String) -> Void
    let openProposal: (String) -> Void

    var body: some View {
        if !held.isEmpty || !proposals.isEmpty {
            VStack(alignment: .leading, spacing: 0) {
                MonacoSectionHeader("In your cabals").padding(.horizontal, MonacoTheme.Space.gutter)
                ForEach(held) { row in
                    Button {
                        openCabal(row.cabalID)
                    } label: {
                        heldRow(row, isLast: proposals.isEmpty && row.id == held.last?.id)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("asset-held-\(row.cabalID)")
                }
                ForEach(proposals) { row in
                    Button {
                        openProposal(row.proposalID)
                    } label: {
                        proposalRow(row, isLast: row.id == proposals.last?.id)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("asset-open-proposal-\(row.proposalID)")
                }
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("asset-in-your-cabals")
        }
    }

    private func heldRow(_ row: AssetHeldRowFacts, isLast: Bool) -> some View {
        MonacoRow(title: row.title, subtitle: row.subtitle, isLast: isLast) {
            cover(id: row.cabalID, pictureURL: row.pictureURL)
        } trailing: {
            Text(row.valueText).moneyFigure(.row).foregroundStyle(MonacoTheme.ink)
            if let note = row.valueNote {
                Text(note).moneyFigure(.caption).foregroundStyle(MonacoTheme.secondaryText)
            } else if let returnText = row.returnText {
                Text(returnText)
                    .moneyFigure(.caption)
                    .foregroundStyle(PnLTone(dollarPnl: "0", percentReturn: returnText).color)
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            [row.title, row.subtitle, row.valueText, row.valueNote ?? row.returnText]
                .compactMap { $0 }.joined(separator: ", "))
    }

    private func proposalRow(_ row: AssetOpenProposalRowFacts, isLast: Bool) -> some View {
        MonacoRow(title: row.title, subtitle: row.subtitle, isLast: isLast) {
            cover(id: row.cabalID, pictureURL: row.pictureURL)
        } trailing: {
            Text(row.trailing).moneyFigure(.caption).foregroundStyle(MonacoTheme.secondaryText)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel([row.title, row.subtitle, row.trailing].joined(separator: ", "))
    }

    private func cover(id: String, pictureURL: String?) -> some View {
        CabalCover(
            cabalId: id, tint: .forGroupId(id), pictureURL: CabalMark.resolvedURL(pictureURL), style: .thumb(40)
        )
        .frame(width: 40, height: 40)
    }
}
