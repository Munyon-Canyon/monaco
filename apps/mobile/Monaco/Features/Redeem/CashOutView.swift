import SwiftUI

struct CashOutView: View {
    let cabalID: String

    enum Copy {
        static let title = "Cash out"
        static let sliceComing = "Your slice shows up here soon."
        static let explainer =
            "We sell this much of your slice and move the cash to your account balance. You stay in the cabal."
        static let submit = "Cash out"
        static let submitComing = "Cash outs open soon."
    }

    var body: some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.l) {
                AmountEntry(
                    amountText: .constant(""),
                    presets: [
                        .fraction(0.25, label: "25%"),
                        .fraction(0.5, label: "50%"),
                        .fraction(1, label: "All"),
                    ]
                )
                .disabled(true)
                .accessibilityIdentifier("cash-out-amount")
                Text(Copy.sliceComing)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .multilineTextAlignment(.center)
                    .accessibilityIdentifier("cash-out-slice-coming")
                AmountEntryNote(Copy.explainer)
                    .accessibilityIdentifier("cash-out-explainer")
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            BottomCTA {
                VStack(spacing: MonacoTheme.Space.s) {
                    Text(Copy.submitComing)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .accessibilityIdentifier("cash-out-submit-coming")
                    Button(Copy.submit) {}
                        .buttonStyle(.monacoPrimary)
                        .disabled(true)
                        .accessibilityIdentifier("cash-out-submit")
                }
            }
        }
        .navigationTitle(Copy.title)
        .navigationBarTitleDisplayMode(.inline)
    }
}

#if DEBUG
final class CashOutSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard arguments.contains("-cashOutHarness") else { return nil }
        return AnyView(NavigationStack { CashOutView(cabalID: "c") }.tint(MonacoTheme.ink))
    }
}
#endif
