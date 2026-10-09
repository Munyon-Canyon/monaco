import MonacoCore
import SwiftUI

struct WithdrawConfirmView: View {
    static let fullBalanceCaption = "Full balance"

    static func fullBalanceFigure(_ availableMicros: Int64) -> Int64 {
        UsdAmountFormatter.flooredToCents(availableMicros)
    }

    static let caveat = "Double-check the address. Transfers can't be undone."
    let destinationAddress: String
    let amountText: String
    let amountLabel: String
    var fullBalanceMicros: Int64?
    let isSubmitting: Bool
    let onWithdraw: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                VStack(alignment: .center, spacing: MonacoTheme.Space.s) {
                    Text("You're withdrawing")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                    if let fullBalanceMicros {
                        MoneyText(micros: Self.fullBalanceFigure(fullBalanceMicros), style: .hero)
                        Text(Self.fullBalanceCaption)
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.muted)
                    } else {
                        MoneyText(decimalString: amountText, style: .hero)
                    }
                }
                .frame(maxWidth: .infinity)
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
                    SubmitLabel(isWorking: isSubmitting, idle: "Withdraw \(amountLabel)", working: "Withdrawing…")
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
