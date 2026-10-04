import SwiftUI

struct WithdrawConfirmView: View {
    static let caveat = "Double-check the address. Transfers can't be undone."
    static let comingSoon = "Withdrawals open soon."

    let destinationAddress: String
    let amountText: String

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    Text("You're withdrawing")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                    MoneyText(decimalString: amountText, style: .large)
                }
                .padding(.horizontal, MonacoTheme.Space.m)
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
                        .padding(.horizontal, MonacoTheme.Space.m)
                }
            }
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                VStack(spacing: MonacoTheme.Space.s) {
                    Text(Self.comingSoon)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                    Button("Withdraw") {}
                        .buttonStyle(.monacoPrimary)
                        .disabled(true)
                }
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("withdraw-submit-coming")
            }
        }
        .navigationTitle("Confirm")
        .navigationBarTitleDisplayMode(.inline)
    }
}
