import MonacoCore
import SwiftUI

struct ProposalReceiptCard: View {
    let model: ProposalDetailModel
    let detail: ProposalDetail
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts

    private var summary: ProposalSummary { detail.summary }
    private var isSell: Bool { summary.kind == "sell" }

    private var pendingUnits: String? {
        guard let atomics = detail.stats.sharesAtomics,
            let units = ProposalResultCopy.units(atomics: atomics, decimals: detail.stats.tokenDecimals)
        else { return nil }
        return "\(units) sh"
    }

    private var notVotedNames: [String] {
        let missing = Set(detail.voters.filter { !$0.voted }.map(\.id))
        return model.members.filter { missing.contains($0.id) }.map(\.name)
    }

    private var hasTrade: Bool {
        switch summary.status {
        case .passed, .executed, .executionBlocked: true
        default: summary.swap != nil
        }
    }

    private var myRow: ProposalResultCopy.Row? {
        ProposalResultCopy.myRow(
            myVote: detail.myVote, choice: summary.myBallot.flatMap(ProposalBallot.init(rawValue:)),
            status: summary.status)
    }

    var body: some View {
        let rows = rowKinds
        if !rows.isEmpty {
            MonacoCard {
                VStack(alignment: .leading, spacing: 0) {
                    ForEach(Array(rows.enumerated()), id: \.offset) { index, kind in
                        if index > 0 { MonacoRule() }
                        row(kind)
                    }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("proposal-receipt")
        }
    }

    private enum RowKind {
        case trade
        case mine
        case notVoted(String)
        case priceMoved(ProposalPriceMove)
        case retry
    }

    private var rowKinds: [RowKind] {
        var kinds: [RowKind] = []
        if hasTrade { kinds.append(.trade) }
        if let move = model.priceMove, summary.swap?.isPriceMoved == true { kinds.append(.priceMoved(move)) }
        if model.canRetry(viewerID: environment.viewer?.userID) { kinds.append(.retry) }
        if myRow != nil { kinds.append(.mine) }
        if let line = ProposalResultCopy.notVotedRow(names: notVotedNames, status: summary.status) {
            kinds.append(.notVoted(line))
        }
        return kinds
    }

    @ViewBuilder private func row(_ kind: RowKind) -> some View {
        switch kind {
        case .trade: tradeRow
        case .mine:
            if let myRow { line(icon: "checkmark.circle", title: myRow.title, subtitle: myRow.subtitle) }
        case .notVoted(let text): line(icon: MonacoIcon.clock.symbol, title: text, subtitle: nil)
        case .priceMoved(let move): priceMoved(move)
        case .retry: retryButton
        }
    }

    @ViewBuilder private var tradeRow: some View {
        if let swap = summary.swap,
            let fill = ProposalResultCopy.tradeRow(swap: swap, isSell: isSell, sinceBps: detail.stats.sinceBps)
        {
            transactionLink(swap.id) {
                line(
                    icon: "arrow.down.to.line", title: fill.title, subtitle: fill.subtitle, trailing: fill.trailing,
                    tone: fill.tone)
            }
        } else {
            let pending = ProposalResultCopy.pendingTradeRow(
                swap: summary.swap, isSell: isSell, units: pendingUnits, statusMessage: summary.statusMessage)
            transactionLink(summary.swap?.id) {
                line(icon: "arrow.down.to.line", title: pending.title, subtitle: pending.subtitle)
            }
        }
    }

    @ViewBuilder private func transactionLink<Content: View>(_ swapID: String?, @ViewBuilder _ content: () -> Content)
        -> some View
    {
        if let swapID {
            NavigationLink(value: AnyAppRoute(TransactionRoute(cabalID: summary.cabalID, transactionID: swapID))) {
                content()
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("proposal-transaction-record")
        } else {
            content()
        }
    }

    private func line(
        icon: String, title: String, subtitle: String?, trailing: String? = nil, tone: ProposalStats.Tone = .plain
    ) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Image(systemName: icon)
                .frame(width: 22, height: 22)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(title).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.ink)
                if let subtitle {
                    Text(subtitle).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.muted)
                }
            }
            Spacer(minLength: MonacoTheme.Space.s)
            if let trailing {
                Text(trailing)
                    .font(MonacoTheme.Typo.headline.monospacedDigit())
                    .foregroundStyle(toneColor(tone))
            }
        }
        .padding(.vertical, MonacoTheme.Space.sm)
        .contentShape(Rectangle())
    }

    private func toneColor(_ tone: ProposalStats.Tone) -> Color {
        switch tone {
        case .gain: MonacoTheme.profit
        case .loss: MonacoTheme.loss
        case .plain: MonacoTheme.muted
        }
    }

    private var retryButton: some View {
        Button(model.retriesAtCurrentPrice ? "Buy at current price" : "Retry") {
            Task {
                await model.retry()
                if model.didRetry {
                    toasts.show(success: "Retrying the trade.")
                } else if let message = model.errorMessage {
                    toasts.current = MonacoToast(message: message)
                }
            }
        }
        .buttonStyle(.monacoSecondary)
        .disabled(model.isRetrying)
        .padding(.vertical, MonacoTheme.Space.sm)
        .accessibilityIdentifier("proposal-retry")
    }

    private func priceMoved(_ move: ProposalPriceMove) -> some View {
        let decimals = model.asset?.decimals ?? AssetCatalogDefaults.decimals
        let kind = model.asset?.kind ?? .stock
        func words(_ quote: Int64) -> String {
            ProposalCardCopy.expected(
                isSell: isSell, quoteOut: quote, usdcMicros: summary.usdcMicros, decimals: decimals, kind: kind) ?? "-"
        }
        return VStack(spacing: 0) {
            ReceiptLine(label: "Voted for", value: .words(words(move.votedQuote)))
            ReceiptLine(label: "Now", value: .words(words(move.currentQuote)))
            ReceiptLine(label: "Price moved", value: .words("\(move.percentText) since the vote"), isLast: true)
        }
        .accessibilityIdentifier("proposal-price-moved")
    }
}
