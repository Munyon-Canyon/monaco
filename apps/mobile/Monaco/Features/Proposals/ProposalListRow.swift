import MonacoCore
import SwiftUI

struct ProposalListRow: Identifiable, Equatable {
    let id: String
    let symbol: String
    let assetName: String?
    let kind: AssetKind
    let logoURL: URL?
    let title: String
    let subtitle: String
    let badge: ProposalBadge

    init(
        id: String, symbol: String, assetName: String?, kind: AssetKind = .stock, logoURL: URL? = nil,
        title: String, subtitle: String, badge: ProposalBadge
    ) {
        self.id = id
        self.symbol = symbol
        self.assetName = assetName
        self.kind = kind
        self.logoURL = logoURL
        self.title = title
        self.subtitle = subtitle
        self.badge = badge
    }

    init(summary: ProposalSummary, asset: ProposalAsset?, proposer: String, now: Date) {
        let state = summary.state(now: now)
        let amount =
            summary.kind == "sell"
            ? ProposalCard.shareLabel(summary: summary, asset: asset)
            : summary.usdcMicros.map { UsdAmountFormatter.format(micros: $0) }
        self.init(
            id: summary.id, symbol: summary.symbol, assetName: asset?.displayName, kind: asset?.kind ?? .stock,
            logoURL: asset?.logoURL,
            title: [summary.assetName(asset: asset), amount].compactMap { $0 }.joined(separator: " · "),
            subtitle: ProposalCardCopy.rowSubtitle(state, proposer: proposer, statusMessage: summary.statusMessage),
            badge: .listRow(state))
    }
}

struct ProposalListRowView: View {
    let row: ProposalListRow
    var isLast = false

    var body: some View {
        NavigationLink(value: AnyAppRoute(ProposalRoute(proposalID: row.id))) {
            MonacoRow(
                title: row.title, subtitle: row.subtitle, chevron: true, isLast: isLast,
                leading: {
                    StockMark(
                        symbol: row.symbol, displayName: row.assetName, assetKind: row.kind,
                        size: MonacoRowLayout.baseMarkSize, logoURL: row.logoURL)
                },
                trailing: { StatusBadge(proposal: row.badge) })
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("proposal-row-\(row.id)")
    }
}
