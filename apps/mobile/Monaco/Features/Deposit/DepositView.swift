import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

struct DepositView: View {
    let prefillMicros: Int64?
    let cabalID: String?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts

    var body: some View {
        @Bindable var deposit = environment.cardDeposit
        DepositChooser(
            creating: deposit.isCreating,
            onCard: { Task { await deposit.start(suggestedMicros: prefillMicros, cabalID: cabalID) } }
        )
        .fullScreenCover(
            item: Binding(get: { deposit.page }, set: { if $0 == nil { deposit.browserClosed() } })
        ) { page in
            SafariView(url: page.url)
                .ignoresSafeArea()
                .onOpenURL { url in
                    guard case .depositComplete(let id) = DeepLink.parse(url) else { return }
                    Task { await deposit.redirected(sessionID: id) }
                }
        }
        .onChange(of: deposit.messageTick) { _, _ in
            guard let message = deposit.message else { return }
            toasts.current = MonacoToast(message: message)
        }
    }
}

struct DepositChooser: View {
    let creating: Bool
    let onCard: () -> Void

    var body: some View {
        ScrollView {
            MonacoGroupedList {
                Button(action: onCard) {
                    MonacoRow(title: "Card", subtitle: "Pay with card or Apple Pay", chevron: !creating) {
                        SunkenGlyphMark(systemImage: "creditcard")
                    } trailing: {
                        if creating {
                            ProgressView()
                                .accessibilityIdentifier("deposit-card-spinner")
                        }
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.monacoRow)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Card")
                .accessibilityHint("Pay with card or Apple Pay")
                .accessibilityAddTraits(.isButton)
                .accessibilityIdentifier("deposit-card-row")

                NavigationLink(value: AnyAppRoute(DepositAddressRoute())) {
                    MonacoRow(title: "Crypto", subtitle: "Send USDC on Solana", chevron: true, isLast: true) {
                        StockMark(symbol: "USDC")
                    } trailing: {
                        EmptyView()
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.monacoRow)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Crypto")
                .accessibilityHint("Send USDC on Solana")
                .accessibilityAddTraits(.isButton)
                .accessibilityIdentifier("deposit-crypto-row")
            }
            .disabled(creating)
            .padding(.top, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Add money")
        .navigationBarTitleDisplayMode(.inline)
    }
}

struct DepositAddressView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: BalanceSource?

    var body: some View {
        DepositContent(
            address: environment.sessionStore.profile?.memberWalletAddress,
            state: model?.state ?? .loading,
            onCopy: copyAddress,
            onRetryAddress: { Task { await environment.sessionStore.reloadProfile(auth: environment.auth) } },
            onRetryBalance: { Task { await model?.load() } }
        )
        .refreshable {
            await environment.sessionStore.reloadProfile(auth: environment.auth)
            await model?.load()
        }
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
        .onChange(of: model?.failureTick) { _, _ in
            guard let error = model?.lastError else { return }
            toasts.current = MonacoToast(message: BalanceSource.message(for: error))
        }
        .onChange(of: model?.balance) { previous, current in
            guard let current, let change = BalanceChange.detect(previous: previous, current: current) else { return }
            toasts.show(success: change.message)
        }
    }

    private func copyAddress(_ address: String) {
        UIPasteboard.general.string = address
        toasts.show(success: "Address copied.")
    }

    private func preparedModel() -> BalanceSource {
        if let model { return model }
        let created = BalanceSource(api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

struct DepositContent: View {
    let address: String?
    let state: LoadState<AccountBalance>
    let onCopy: (String) -> Void
    let onRetryAddress: () -> Void
    let onRetryBalance: () -> Void

    enum Section: Hashable { case balance, address, howItWorks }
    static let order: [Section] = [.balance, .address, .howItWorks]

    static let steps = [
        "Send USDC on Solana",
        "It lands in your balance",
        "Fund a cabal",
    ]

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                ForEach(Self.order, id: \.self) { section in
                    switch section {
                    case .balance:
                        MonacoGroupedList {
                            PlatformBalanceCard(state: state, valueIdentifier: "deposit-screen-balance-value")
                            if case .failed = state {
                                balanceFailure
                            }
                        }
                    case .address:
                        DepositAddressCard(
                            content: .resolve(address: address), onCopy: onCopy, onRetry: onRetryAddress
                        )
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                    case .howItWorks:
                        howItWorks
                    }
                }
            }
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
        .navigationTitle("Add money")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var balanceFailure: some View {
        MonacoErrorRow(thing: "your balance", identifier: "deposit-balance-error", retry: onRetryBalance)
    }

    private var howItWorks: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("How it works")
            let layout =
                dynamicTypeSize.isAccessibilitySize
                ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.s))
                : AnyLayout(HStackLayout(alignment: .top, spacing: MonacoTheme.Space.s))
            layout {
                ForEach(Array(Self.steps.enumerated()), id: \.offset) { index, step in
                    stepView(number: index + 1, label: step)
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("deposit-how-it-works")
    }

    private func stepView(number: Int, label: String) -> some View {
        let stacked = dynamicTypeSize.isAccessibilitySize
        let inner =
            (stacked
                ? AnyLayout(HStackLayout(alignment: .center, spacing: MonacoTheme.Space.sm))
                : AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs)))
        return inner {
            Text("\(number)")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.ink)
                .frame(width: 24, height: 24)
                .background(Circle().fill(MonacoTheme.muted.opacity(0.15)))
                .accessibilityHidden(true)
            Text(label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Step \(number): \(label)")
    }
}

struct DepositAddressCard: View {
    enum Content: Equatable {
        case loading
        case ready(String)
        case unavailable

        static func resolve(address: String?) -> Content {
            DepositAddress.usable(address).map(Content.ready) ?? .unavailable
        }
    }

    let content: Content
    var addressIdentifier = "deposit-address-value"
    var copyIdentifier = "deposit-address-copy-button"
    let onCopy: (String) -> Void
    let onRetry: () -> Void

    static let networkNote = "Only send USDC on Solana to this address."

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            Text("Your deposit address")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.muted)

            switch content {
            case .loading:
                loading
            case .ready(let address):
                ready(address)
            case .unavailable:
                unavailable
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoSurfaceCard()
    }

    private func ready(_ address: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            DepositQRCode(address: address, onCopy: onCopy)
                .frame(maxWidth: .infinity)

            MonacoWalletAddressText(address: address)
                .accessibilityIdentifier(addressIdentifier)
                .onTapGesture {
                    onCopy(address)
                }

            VStack(spacing: MonacoTheme.Space.s) {
                Button("Copy address") {
                    onCopy(address)
                }
                .buttonStyle(.monacoPrimary)
                .monacoFullWidthButtons()
                .accessibilityIdentifier(copyIdentifier)

                HStack(spacing: MonacoTheme.Space.s) {
                    Image("SolanaMark")
                        .resizable()
                        .scaledToFit()
                        .frame(width: 20)
                        .accessibilityHidden(true)
                    Text(Self.networkNote)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .multilineTextAlignment(.leading)
                }
                .frame(maxWidth: .infinity)
            }
        }
    }

    private var loading: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            SkeletonBlock(width: 200, height: 200, radius: MonacoTheme.Radius.card)
                .frame(maxWidth: .infinity)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                SkeletonBlock(height: 16)
                SkeletonBlock(width: 120, height: 16)
            }
            SkeletonBlock(height: MonacoButtonMetrics.minimumHeight, radius: MonacoButtonMetrics.minimumHeight / 2)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading your deposit address")
        .accessibilityIdentifier("deposit-address-loading")
    }

    private var unavailable: some View {
        MonacoErrorRow(thing: "your deposit address", identifier: "deposit-address-error", retry: onRetry)
    }
}
