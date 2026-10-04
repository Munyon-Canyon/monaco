import MonacoAPI
import MonacoCore
import SwiftUI
import UIKit

struct DepositView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: BalanceSource?

    var body: some View {
        DepositContent(
            state: model?.state ?? .loading,
            onCopy: copyAddress,
            onRetry: { Task { await model?.load() } }
        )
        .refreshable { await model?.load() }
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
    let state: LoadState<AccountBalance>
    let onCopy: (String) -> Void
    let onRetry: () -> Void

    static let steps = [
        "Send USDC to the address above from an exchange or another app.",
        "Your account balance updates a few seconds after it arrives.",
        "Fund a cabal to move it into the pot and grow your slice.",
    ]

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                DepositAddressCard(content: .resolve(state), onCopy: onCopy, onRetry: onRetry)
                    .padding(.horizontal, MonacoTheme.Space.m)

                MonacoGroupedList {
                    PlatformBalanceCard(state: state, valueIdentifier: "deposit-screen-balance-value")
                }

                howItWorks
            }
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
        .navigationTitle("Add money")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var howItWorks: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("How it works")
                .padding(.horizontal, MonacoTheme.Space.m)

            MonacoGroupedList {
                ForEach(Array(Self.steps.enumerated()), id: \.offset) { index, step in
                    Text(step)
                        .font(MonacoTheme.Typo.callout)
                        .foregroundStyle(MonacoTheme.ink)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.horizontal, MonacoTheme.Space.m)
                        .padding(.vertical, MonacoTheme.Space.sm)
                        .frame(minHeight: 52)
                        .overlay(alignment: .bottom) {
                            if index < Self.steps.count - 1 {
                                MonacoRule()
                                    .padding(.leading, MonacoTheme.Space.m)
                            }
                        }
                }
            }
        }
    }
}

struct DepositAddressCard: View {
    enum Content: Equatable {
        case loading
        case ready(String)
        case unavailable(String)

        static let loadFailure = "Couldn't load your deposit address."

        static func resolve(_ state: LoadState<AccountBalance>) -> Content {
            switch state {
            case .idle, .loading:
                .loading
            case .loaded(let balance):
                DepositAddress.usable(balance.depositAddress).map(Content.ready) ?? .unavailable(loadFailure)
            case .failed:
                .unavailable(loadFailure)
            }
        }
    }

    let content: Content
    var addressIdentifier = "deposit-address-value"
    var copyIdentifier = "deposit-address-copy-button"
    let onCopy: (String) -> Void
    var onRetry: (() -> Void)?

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
            case .unavailable(let message):
                unavailable(message)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoSurfaceCard()
    }

    private func ready(_ address: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
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

                Text(Self.networkNote)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: .infinity)
            }
        }
    }

    private var loading: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
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

    private func unavailable(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            Text(message)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .fixedSize(horizontal: false, vertical: true)
            if let onRetry {
                Button("Try again", action: onRetry)
                    .buttonStyle(.monacoSecondary)
                    .monacoFullWidthButtons()
                    .accessibilityIdentifier("deposit-address-retry")
            }
        }
    }
}
