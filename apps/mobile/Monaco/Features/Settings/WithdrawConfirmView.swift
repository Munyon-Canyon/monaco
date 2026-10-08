import SwiftUI

struct WithdrawConfirmView: View {
    static let caveat = "Double-check the address. Transfers can't be undone."
    let destinationAddress: String
    let amountText: String
    var fullBalanceLabel: String?
    let isSubmitting: Bool
    let onWithdraw: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    Text("You're withdrawing")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                    if let fullBalanceLabel {
                        Text(fullBalanceLabel)
                            .font(MonacoTheme.Typo.moneyLarge)
                            .foregroundStyle(MonacoTheme.ink)
                    } else {
                        MoneyText(decimalString: amountText, style: .large)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityElement(children: .combine)

                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    MonacoGroupedList {
                        ReceiptLine(label: "To", value: .address(destinationAddress))
                        ReceiptLine(label: "From", value: .words("Account balance"))
                        ReceiptLine(label: "Arrives", value: .words("About a minute"), isLast: true)
                    }
                    Text(Self.caveat)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                }
            }
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                Button(action: onWithdraw) {
                    HStack(spacing: MonacoTheme.Space.s) {
                        if isSubmitting {
                            ProgressView().tint(MonacoTheme.primaryButtonLabel)
                            Text("Withdrawing…")
                        } else {
                            Text("Withdraw")
                        }
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(.monacoPrimary)
                .disabled(isSubmitting)
                .accessibilityIdentifier("withdraw-confirm-button")
            }
        }
        .navigationBarBackButtonHidden(isSubmitting)
        .navigationTitle("Confirm")
        .navigationBarTitleDisplayMode(.inline)
    }
}
