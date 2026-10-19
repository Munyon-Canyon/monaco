import MonacoCore
import SwiftUI

enum WithdrawOutcomeStatus: Equatable {
    case pending
    case completed
    case failed
}

struct WithdrawOutcomeSummary: Equatable {
    let amountLabel: String
    var status: WithdrawOutcomeStatus
}

struct WithdrawConfirmView: View {
    static let fullBalanceCaption = "Full balance"

    static func fullBalanceFigure(_ availableMicros: Int64) -> Int64 {
        UsdAmountFormatter.flooredToCents(availableMicros)
    }

    static let caveat = "Double-check the address and network. Transfers can't be undone."
    let destinationAddress: String
    var network = WithdrawNetwork.solana
    let amountText: String
    let amountLabel: String
    var fullBalanceMicros: Int64?
    var networkCost: String?
    var outcome: WithdrawOutcomeSummary?
    let isSubmitting: Bool
    let onWithdraw: () -> Void
    var onViewTransaction: () -> Void = {}

    private var displayAmount: String {
        if let fullBalanceMicros {
            return WithdrawAmount.fullBalanceLabel(micros: Self.fullBalanceFigure(fullBalanceMicros))
        }
        return amountLabel
    }

    var body: some View {
        Group {
            if let outcome {
                outcomeView(outcome)
            } else {
                review
            }
        }
        .navigationBarBackButtonHidden(isSubmitting)
        .navigationTitle("Withdraw to wallet")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var review: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                Text("Review withdrawal")
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.ink)
                    .padding(.vertical, MonacoTheme.Space.m)
                    .accessibilityAddTraits(.isHeader)
                MoneyKeyRowCard(rows: [
                    MoneyKeyRow(label: "From", value: .words("Your cash")),
                    MoneyKeyRow(label: "Asset", value: .words("USDC")),
                    MoneyKeyRow(label: "Amount", value: .data(displayAmount)),
                    MoneyKeyRow(label: "Network", value: .words(network.name)),
                    MoneyKeyRow(label: "Destination", value: .address(destinationAddress)),
                    MoneyKeyRow(label: "Arrives", value: .words("About a minute")),
                    MoneyKeyRow(label: "Network costs", value: .words(networkCost ?? "Included")),
                ])
                MonacoNotice(.warn, message: Self.caveat)
                    .accessibilityIdentifier("withdraw-caveat")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button(action: onWithdraw) {
                    SubmitLabel(isWorking: isSubmitting, idle: "Withdraw", working: "Withdrawing…")
                }
                .buttonStyle(.monacoPrimary)
                .disabled(isSubmitting)
                .accessibilityIdentifier("withdraw-confirm-button")
            }
        }
    }

    private func outcomeView(_ outcome: WithdrawOutcomeSummary) -> some View {
        let rows = [
            MoneyKeyRow(label: "Amount", value: .data("\(outcome.amountLabel) USDC")),
            MoneyKeyRow(label: "Network", value: .words(network.name)),
            MoneyKeyRow(label: "Destination", value: .address(destinationAddress)),
            MoneyKeyRow(label: "Status", value: .words(Self.statusWord(outcome.status))),
        ]
        return MoneyOutcome(
            tone: Self.tone(outcome.status), title: Self.title(outcome.status), message: Self.message(outcome.status),
            rows: rows, identifier: "withdraw-outcome"
        ) {
            Button("View transaction", action: onViewTransaction)
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("withdraw-view-transaction-button")
        }
    }

    static func statusWord(_ status: WithdrawOutcomeStatus) -> String {
        switch status {
        case .pending: "Pending"
        case .completed: "Completed"
        case .failed: "Failed"
        }
    }

    static func tone(_ status: WithdrawOutcomeStatus) -> MoneyOutcomeTone {
        switch status {
        case .pending: .pending
        case .completed: .done
        case .failed: .error
        }
    }

    static func title(_ status: WithdrawOutcomeStatus) -> String {
        switch status {
        case .pending: "Withdrawal processing"
        case .completed: "Withdrawal completed"
        case .failed: "Withdrawal didn't go through"
        }
    }

    static func message(_ status: WithdrawOutcomeStatus) -> String {
        switch status {
        case .pending: "Your request is being processed. Check its status before submitting another transfer."
        case .completed: "The network transfer is confirmed."
        case .failed: WithdrawProgress.didNotGoThrough
        }
    }
}
