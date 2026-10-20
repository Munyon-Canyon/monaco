import MonacoCore
import SwiftUI

struct ProposalStatusSection: View {
    let model: ProposalDetailModel
    let summary: ProposalSummary
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts

    private var state: ProposalState { summary.state(now: .now) }

    private var stepperLabel: String {
        ProposalStepper.make(
            status: summary.status, isSell: summary.kind == "sell", swapFailed: summary.swap?.status == "failed",
            expiresAt: summary.expiresAt, failureMessage: summary.swap?.failureMessage,
            statusMessage: summary.statusMessage, priceMoved: summary.swap?.isPriceMoved == true
        ).accessibilityLabel
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Status")
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                if let notice = ProposalCardCopy.statusNotice(
                    state, failureMessage: summary.swap?.failureMessage, statusMessage: summary.statusMessage)
                {
                    MonacoNotice(proposal: notice)
                }
                if let move = model.priceMove, summary.swap?.isPriceMoved == true {
                    priceMoved(move)
                }
                if model.canRetry(viewerID: environment.viewer?.userID) {
                    retryButton
                }
                if ProposalExecutionDetails.applies(to: state) {
                    ProposalExecutionDetails(summary: summary, state: state, kind: model.asset?.kind ?? .stock)
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(stepperLabel)
        .accessibilityIdentifier("proposal-tracker")
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
        .accessibilityIdentifier("proposal-retry")
    }

    private func priceMoved(_ move: ProposalPriceMove) -> some View {
        let isSell = summary.kind == "sell"
        let decimals = model.asset?.decimals ?? AssetCatalogDefaults.decimals
        let kind = model.asset?.kind ?? .stock
        func words(_ quote: Int64) -> String {
            ProposalCardCopy.expected(
                isSell: isSell, quoteOut: quote, usdcMicros: summary.usdcMicros, decimals: decimals, kind: kind) ?? "-"
        }
        return MonacoCard(style: .inset) {
            VStack(spacing: 0) {
                ReceiptLine(label: "Voted for", value: .words(words(move.votedQuote)))
                ReceiptLine(label: "Now", value: .words(words(move.currentQuote)))
                ReceiptLine(label: "Price moved", value: .words("\(move.percentText) since the vote"), isLast: true)
            }
        }
        .accessibilityIdentifier("proposal-price-moved")
    }
}

struct ProposalExecutionDetails: View {
    let summary: ProposalSummary
    let state: ProposalState
    var kind: AssetKind = .stock
    @State private var expanded = false

    static func applies(to state: ProposalState) -> Bool {
        switch state {
        case .approved, .executed, .blocked: true
        case .open, .voted, .viewOnly, .expired, .rejected, .withdrawn, .voided: false
        }
    }

    private var network: String {
        summary.network.map(BestPrice.chainName) ?? "Selected at execution"
    }

    var body: some View {
        DisclosureGroup(isExpanded: $expanded) {
            VStack(spacing: 0) {
                MonacoRule()
                ReceiptLine(label: "Status", value: .words(ProposalBadge.hero(state).label))
                if state == .executed {
                    let facts = summary.swap?.executionFigures(kind: kind)
                    ReceiptLine(label: "Executed price", value: .words(facts?.price ?? "Not available"))
                    ReceiptLine(label: "Units", value: .words(facts?.units ?? "Not available"))
                } else {
                    ReceiptLine(label: "Executed price", value: .words("Not confirmed"))
                }
                ReceiptLine(label: "Network", value: .words(network), isLast: summary.swap?.id == nil)
                if let swapID = summary.swap?.id {
                    transactionRecord(swapID)
                }
            }
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                Image(systemName: MonacoIcon.info.symbol)
                    .accessibilityHidden(true)
                Text("Execution details")
            }
            .foregroundStyle(expanded ? MonacoTheme.ink : MonacoTheme.muted)
        }
        .disclosureGroupStyle(.monaco)
    }

    private func transactionRecord(_ swapID: String) -> some View {
        NavigationLink(value: AnyAppRoute(TransactionRoute(cabalID: summary.cabalID, transactionID: swapID))) {
            HStack(spacing: MonacoTheme.Space.sm) {
                SunkenGlyphMark(icon: .receipt, size: MonacoRowLayout.baseMarkSize)
                Text("Transaction record")
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.ink)
                Spacer(minLength: MonacoTheme.Space.s)
                MonacoRowChevron()
            }
            .padding(.vertical, MonacoTheme.Space.sm)
            .frame(minHeight: ReceiptLineMetrics.minHeight)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("proposal-transaction-record")
    }
}
