import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI
import UIKit

struct DepositView: View {
    let prefillMicros: Int64?
    let cabalID: String?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var addresses: DepositAddressesSource?

    var body: some View {
        @Bindable var deposit = environment.cardDeposit
        DepositChooser(
            creating: deposit.isCreating,
            networks: DepositAddressesSource.offered(
                addresses?.state ?? .idle, solanaAddress: environment.sessionStore.profile?.memberWalletAddress),
            onCard: { Task { await deposit.start(suggestedMicros: prefillMicros, cabalID: cabalID) } }
        )
        .task(id: environment.sessionStore.profile?.userID) {
            let source = DepositAddressesSource(api: environment.api)
            addresses = source
            await source.load()
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

struct DepositChooser: View {
    let creating: Bool
    var networks = [DepositNetwork(id: DepositNetwork.solanaID, address: "")]
    let onCard: () -> Void

    static let cardHint = "Card or Apple Pay. You choose the amount next."
    static let cryptoTitle = "Send crypto"

    static func cryptoHint(_ networks: [DepositNetwork]) -> String {
        let names = networks.map(\.displayName)
        guard let last = names.last else { return "Send USDC" }
        guard names.count > 1 else { return "Send USDC on \(last)" }
        return names.dropLast().joined(separator: ", ") + " or " + last
    }

    static func cryptoRoute(_ networks: [DepositNetwork]) -> AnyAppRoute {
        guard networks.count > 1 else { return AnyAppRoute(DepositAddressRoute(network: networks.first)) }
        return AnyAppRoute(DepositNetworksRoute(networks: networks))
    }

    enum Row: Hashable { case onramp, crypto }
    static let order: [Row] = [.onramp, .crypto]

    var body: some View {
        ScrollView {
            MonacoGroupedList {
                ForEach(Self.order, id: \.self) { row in
                    switch row {
                    case .onramp: onrampRow
                    case .crypto: cryptoRow
                    }
                }
            }
            .disabled(creating)
            .padding(.top, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Add money")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var onrampRow: some View {
        Button(action: onCard) {
            MonacoRow(title: "Card", subtitle: Self.cardHint, chevron: !creating) {
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
        .accessibilityHint(Self.cardHint)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("deposit-card-row")
    }

    private var cryptoRow: some View {
        NavigationLink(value: Self.cryptoRoute(networks)) {
            MonacoRow(title: Self.cryptoTitle, subtitle: Self.cryptoHint(networks), chevron: true, isLast: true) {
                StockMark(symbol: "USDC")
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.monacoRow)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.cryptoTitle)
        .accessibilityHint(Self.cryptoHint(networks))
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("deposit-crypto-row")
    }
}

private struct DepositAddressKey: Hashable {
    let userID: String?
    let network: String
}

private struct AddressPoll: Hashable {
    let address: String?
    let retry: Int
}

struct DepositNetworksView: View {
    let networks: [DepositNetwork]

    static let title = "Choose a network"

    var body: some View {
        ScrollView {
            MonacoGroupedList {
                ForEach(Array(networks.enumerated()), id: \.element.id) { index, network in
                    NavigationLink(value: AnyAppRoute(DepositAddressRoute(network: network))) {
                        MonacoRow(
                            title: network.displayName, subtitle: DepositAddressCard.arrivalNote(for: network),
                            chevron: true, isLast: index == networks.count - 1
                        ) {
                            DepositNetworkMark(network: network)
                        }
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(network.displayName)
                    .accessibilityAddTraits(.isButton)
                    .accessibilityIdentifier("deposit-network-\(network.id)")
                }
            }
            .padding(.top, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle(Self.title)
        .navigationBarTitleDisplayMode(.inline)
    }
}

struct DepositNetworkMark: View {
    let network: DepositNetwork

    var body: some View {
        ChainMark(network: network.id)
    }
}

struct DepositAddressView: View {
    var network: DepositNetwork?

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var addressAttempts = 0
    @State private var addressRetry = 0

    private static let maxAddressAttempts = 5

    private var shown: DepositNetwork {
        network ?? DepositNetwork(id: DepositNetwork.solanaID, address: "")
    }

    private var address: String? {
        shown.isSolana ? environment.sessionStore.profile?.memberWalletAddress : shown.address
    }

    var body: some View {
        DepositContent(
            address: address,
            network: shown,
            gaveUp: addressAttempts >= Self.maxAddressAttempts,
            state: environment.balance.state,
            onCopy: copyAddress,
            onRetryAddress: {
                addressAttempts = 0
                addressRetry += 1
            },
            onRetryBalance: { Task { await environment.balance.load() } }
        )
        .id(DepositAddressKey(userID: environment.sessionStore.profile?.userID, network: shown.id))
        .refreshable {
            await environment.sessionStore.reloadProfile(auth: environment.auth)
            await environment.balance.load()
        }
        .task(id: AddressPoll(address: address, retry: addressRetry)) {
            while shown.isSolana, DepositAddress.usable(environment.sessionStore.profile?.memberWalletAddress) == nil,
                addressAttempts < Self.maxAddressAttempts
            {
                await environment.sessionStore.reloadProfile(auth: environment.auth)
                addressAttempts += 1
                try? await Task.sleep(for: .seconds(2))
                if Task.isCancelled { return }
            }
        }
        .task {
            await environment.balance.load()
        }
    }

    private func copyAddress(_ address: String) {
        UIPasteboard.general.string = address
        AppAnalytics.current.step(.cryptoDeposit(.addressCopied))
        toasts.show(success: "Address copied.")
    }
}

struct DepositContent: View {
    let address: String?
    var network = DepositNetwork(id: DepositNetwork.solanaID, address: "")
    var gaveUp = false
    let state: LoadState<AccountBalance>
    let onCopy: (String) -> Void
    let onRetryAddress: () -> Void
    let onRetryBalance: () -> Void

    enum Section: Hashable { case balance, address, howItWorks }
    static let order: [Section] = [.balance, .address, .howItWorks]

    static let steps = steps(on: "Solana")

    static func steps(on network: String) -> [String] {
        ["Send USDC on \(network)", "It lands in your balance", "Fund a cabal"]
    }

    static func title(on network: String) -> String {
        "Send USDC on \(network)"
    }

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
                        let content = DepositAddressCard.Content.resolve(address: address, gaveUp: gaveUp)
                        if content == .unavailable {
                            MonacoErrorRow(
                                thing: "your deposit address", identifier: "deposit-address-error",
                                retry: onRetryAddress)
                        } else {
                            DepositAddressCard(content: content, network: network, onCopy: onCopy)
                                .padding(.horizontal, MonacoTheme.Space.gutter)
                        }
                    case .howItWorks:
                        howItWorks
                    }
                }
            }
            .padding(.top, MonacoTheme.Space.m)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
        .navigationTitle(Self.title(on: network.displayName))
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
                ForEach(Array(Self.steps(on: network.displayName).enumerated()), id: \.offset) { index, step in
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
                .background(Circle().fill(MonacoTheme.surfaceSunken))
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

        static func resolve(address: String?, gaveUp: Bool) -> Content {
            if let usable = DepositAddress.usable(address) { return .ready(usable) }
            let placeholder = address?.trimmingCharacters(in: .whitespacesAndNewlines).hasPrefix("FAKE") == true
            return placeholder || gaveUp ? .unavailable : .loading
        }
    }

    let content: Content
    var network = DepositNetwork(id: DepositNetwork.solanaID, address: "")
    var addressIdentifier = "deposit-address-value"
    var copyIdentifier = "deposit-address-copy-button"
    let onCopy: (String) -> Void

    static let networkNote = networkNote(on: "Solana")
    static let evmArrival = "Arrives in a few minutes"

    static func networkNote(on network: String) -> String {
        "Only send USDC on \(network) to this address."
    }

    static func arrivalNote(for network: DepositNetwork) -> String? {
        network.isEVM ? evmArrival : nil
    }

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
                EmptyView()
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoActionCard()
    }

    private func ready(_ address: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            DepositQRCode(address: address, showsSolanaMark: network.isSolana, onCopy: onCopy)
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
                .accessibilityIdentifier(copyIdentifier)

                HStack(spacing: MonacoTheme.Space.s) {
                    if network.isSolana {
                        Image("SolanaMark")
                            .resizable()
                            .scaledToFit()
                            .frame(width: 20)
                            .accessibilityHidden(true)
                    } else {
                        Image(systemName: "exclamationmark.triangle")
                            .foregroundStyle(MonacoTheme.muted)
                            .accessibilityHidden(true)
                    }
                    Text(Self.networkNote(on: network.displayName))
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .multilineTextAlignment(.leading)
                        .accessibilityIdentifier("deposit-network-warning")
                }
                .frame(maxWidth: .infinity)

                if let arrival = Self.arrivalNote(for: network) {
                    Text(arrival)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                        .frame(maxWidth: .infinity)
                        .accessibilityIdentifier("deposit-arrival-note")
                }
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
}
