import MonacoCore
import SwiftUI

enum CardFundingScreen: Equatable {
    case handoff(working: Bool)
    case checking
    case received
    case cancelled(String)

    static func resolve(_ state: CardDepositState) -> CardFundingScreen {
        switch state {
        case .idle, .browsing: .handoff(working: false)
        case .creating: .handoff(working: true)
        case .processing: .checking
        case .done: .received
        case .failed(.some, _): .cancelled(CardDepositState.didNotGoThrough)
        case .failed(nil, _): .handoff(working: false)
        }
    }
}

struct CardFundingView: View {
    let prefillMicros: Int64?
    let cabalID: String?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @State private var baseline: CardDepositState?

    var body: some View {
        @Bindable var deposit = environment.cardDeposit
        CardFundingContent(
            screen: CardFundingScreen.resolve(deposit.state == baseline ? .idle : deposit.state),
            onContinue: { Task { await deposit.start(suggestedMicros: prefillMicros, cabalID: cabalID) } },
            onBack: { dismiss() }
        )
        .toolbar(.hidden, for: .tabBar)
        .onAppear {
            if baseline == nil { baseline = deposit.state }
        }
        .fullScreenCover(
            item: Binding(get: { deposit.page }, set: { if $0 == nil { deposit.browserClosed() } })
        ) { page in
            SafariView(url: page.url)
                .ignoresSafeArea()
        }
        .onChange(of: deposit.messageTick) { _, _ in
            guard let message = deposit.message else { return }
            toasts.current = MonacoToast(message: message)
        }
    }
}

struct CardFundingContent: View {
    let screen: CardFundingScreen
    let onContinue: () -> Void
    let onBack: () -> Void

    static let helper = "Card and Apple Pay deposits continue with the existing funding provider."
    static let providerNotice =
        "The deposit provider confirms your eligibility, payment methods and final fees before payment."
    static let checkingMessage =
        "You returned from the funding provider. We'll update your balance only after confirmation."

    var body: some View {
        Group {
            switch screen {
            case .handoff(let working):
                handoff(working: working)
            case .checking:
                MoneyOutcome(
                    tone: .pending, title: "Checking your deposit", message: Self.checkingMessage,
                    identifier: "card-funding-returned"
                ) {
                    viewCash
                }
            case .received:
                MoneyOutcome(
                    tone: .done, title: "Deposit received", message: "Your cash balance is updated.",
                    identifier: "card-funding-received"
                ) {
                    viewCash
                }
            case .cancelled(let message):
                MoneyOutcome(
                    tone: .pending, title: "Funding cancelled", message: message,
                    identifier: "card-funding-cancelled"
                ) {
                    Button("Back to funding methods", action: onBack)
                        .buttonStyle(.monacoPrimary)
                        .accessibilityIdentifier("card-funding-back-button")
                }
            }
        }
        .navigationTitle("Card / Apple Pay")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var viewCash: some View {
        Button("View cash", action: onBack)
            .buttonStyle(.monacoPrimary)
            .accessibilityIdentifier("card-funding-view-cash-button")
    }

    private func handoff(working: Bool) -> some View {
        ScrollView {
            VStack(spacing: MonacoTheme.Space.l) {
                VStack(spacing: MonacoTheme.Space.m) {
                    Image(systemName: "creditcard")
                        .font(MonacoTheme.Typo.title)
                        .foregroundStyle(MonacoTheme.brand)
                        .frame(width: OutcomeMetrics.orb, height: OutcomeMetrics.orb)
                        .background(
                            RoundedRectangle(cornerRadius: OutcomeMetrics.orbRadius, style: .continuous)
                                .fill(MonacoTheme.brandSoft)
                        )
                        .accessibilityHidden(true)
                    Text("Continue securely.")
                        .font(MonacoTheme.Typo.title)
                        .foregroundStyle(MonacoTheme.ink)
                        .multilineTextAlignment(.center)
                        .accessibilityAddTraits(.isHeader)
                    Text(Self.helper)
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.muted)
                        .multilineTextAlignment(.center)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: OutcomeMetrics.messageWidth)
                }
                MoneyKeyRowCard(rows: [
                    MoneyKeyRow(label: "Destination", value: .words("Your cash balance")),
                    MoneyKeyRow(label: "Amount & fees", value: .words("Shown by the provider")),
                    MoneyKeyRow(label: "Next step", value: .words("Provider sign-in / checkout")),
                ])
                MonacoNotice(.info, message: Self.providerNotice)
                VStack(spacing: MonacoTheme.Space.sm) {
                    Button(working ? "Opening…" : "Continue to provider", action: onContinue)
                        .buttonStyle(.monacoPrimary)
                        .disabled(working)
                        .accessibilityIdentifier("deposit-card-spinner")
                    Button("Choose another method", action: onBack)
                        .buttonStyle(.monacoOutline)
                        .disabled(working)
                        .accessibilityIdentifier("card-funding-choose-another-button")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.xl)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .accessibilityIdentifier("card-funding-handoff")
    }
}
