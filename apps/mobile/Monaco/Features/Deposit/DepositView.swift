import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI
import UIKit

struct DepositView: View {
    let prefillMicros: Int64?
    let cabalID: String?

    @Environment(AppEnvironment.self) private var environment
    @State private var addresses: DepositAddressesSource?

    var body: some View {
        DepositChooser(
            state: environment.balance.state,
            networks: DepositAddressesSource.offered(
                addresses?.state ?? .idle, solanaAddress: environment.sessionStore.profile?.memberWalletAddress),
            cardStartFailed: Self.cardStartFailed(environment.cardDeposit.state),
            cardRoute: CardFundingRoute(prefillMicros: prefillMicros, cabalID: cabalID),
            onRetryBalance: { Task { await environment.balance.load() } }
        )
        .task(id: environment.sessionStore.profile?.userID) {
            let source = DepositAddressesSource(api: environment.api)
            addresses = source
            await source.load()
        }
        .task {
            await environment.balance.load()
        }
    }

    static func cardStartFailed(_ state: CardDepositState) -> Bool {
        if case .failed(nil, _) = state { return true }
        return false
    }
}

struct DepositChooser: View {
    let state: LoadState<AccountBalance>
    var networks = [DepositNetwork(id: DepositNetwork.solanaID, address: "")]
    var cardStartFailed = false
    var cardRoute = CardFundingRoute(prefillMicros: nil, cabalID: nil)
    var onRetryBalance: () -> Void = {}

    static let cardTitle = "Card or Apple Pay"
    static let cardHint = "Continue to the deposit provider"
    static let cryptoTitle = "Send crypto"
    static let unavailableNotice = "Some funding methods may be unavailable in your location or account state."
    static let arrivalNotice = "Money arrives in your cash balance. Funding a cabal is a separate step."

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

    private var display: HomeBalanceDisplay { .resolve(state) }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                hero
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.m)
                if case .failed = state {
                    MonacoErrorRow(thing: "your balance", identifier: "deposit-balance-error", retry: onRetryBalance)
                }
                MonacoSectionHeader("How would you like to deposit?")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.section)
                    .padding(.bottom, MonacoTheme.Space.s)
                MonacoGroupedList {
                    ForEach(Self.order, id: \.self) { row in
                        switch row {
                        case .onramp: onrampRow
                        case .crypto: cryptoRow
                        }
                    }
                }
                VStack(spacing: MonacoTheme.Space.sm) {
                    if cardStartFailed {
                        MonacoNotice(.warn, message: Self.unavailableNotice)
                            .accessibilityIdentifier("deposit-unavailable-notice")
                    }
                    MonacoNotice(.info, message: Self.arrivalNotice)
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.vertical, MonacoTheme.Space.sm)
            }
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .monacoCanvas()
        .navigationTitle("Deposit")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var hero: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text("Your cash")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.muted)
            switch display {
            case .loading:
                SkeletonBlock(width: 132, height: 36)
                    .accessibilityIdentifier("platform-balance-loading")
            case .amount(let micros):
                MoneyText(micros: UsdAmountFormatter.flooredToCents(micros), style: .large)
                    .foregroundStyle(MonacoTheme.ink)
                    .accessibilityIdentifier("deposit-screen-balance-value")
            case .unavailable:
                Text("—")
                    .moneyFont(.large)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityLabel("Account balance unavailable")
                    .accessibilityIdentifier("platform-balance-unavailable")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var onrampRow: some View {
        NavigationLink(value: AnyAppRoute(cardRoute)) {
            MonacoRow(title: Self.cardTitle, subtitle: Self.cardHint, chevron: true) {
                SunkenGlyphMark(systemImage: "creditcard")
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.monacoRow)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.cardTitle)
        .accessibilityHint(Self.cardHint)
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("deposit-card-row")
    }

    private var cryptoRow: some View {
        NavigationLink(value: Self.cryptoRoute(networks)) {
            MonacoRow(title: Self.cryptoTitle, subtitle: "USDC on a supported network", chevron: true, isLast: true) {
                SunkenGlyphMark(systemImage: "wallet.bifold")
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
    static let helper =
        "Choose the network you will send USDC from. Use only the matching address provided for that network."
    static let warning = "Do not send unsupported assets or use a different network for the address."
    static let solanaSubtitle = "Direct USDC deposit"

    static func subtitle(for network: DepositNetwork) -> String? {
        network.isSolana ? solanaSubtitle : DepositAddressCard.arrivalNote(for: network)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Text(Self.helper)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.vertical, MonacoTheme.Space.m)
                MonacoGroupedList {
                    ForEach(Array(networks.enumerated()), id: \.element.id) { index, network in
                        NavigationLink(value: AnyAppRoute(DepositAddressRoute(network: network))) {
                            MonacoRow(
                                title: network.displayName, subtitle: Self.subtitle(for: network),
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
                MonacoNotice(.info, message: Self.warning)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.vertical, MonacoTheme.Space.sm)
            }
            .padding(.bottom, MonacoTheme.Space.l)
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
    @State private var received: Int64?

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
            onRetryBalance: { Task { await environment.balance.load() } },
            receivedMicros: received,
            onFundCabal: { environment.navigator.selectedTab = .cabals },
            onHome: {
                environment.navigator.homePath = []
                environment.navigator.selectedTab = .home
            }
        )
        .onChange(of: environment.balance.balance) { previous, current in
            guard let current, case .deposited(let micros)? = BalanceChange.detect(previous: previous, current: current)
            else { return }
            received = micros
        }
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
    var receivedMicros: Int64?
    var onFundCabal: () -> Void = {}
    var onHome: () -> Void = {}

    enum Section: Hashable { case head, address, notices, howItWorks }
    static let order: [Section] = [.head, .address, .notices, .howItWorks]

    static let steps = steps(on: "Solana")
    static let waitingNotice = "Waiting for network confirmation. Your balance hasn't been credited yet."

    static func steps(on network: String) -> [String] {
        ["Send USDC on \(network)", "It lands in your balance", "Fund a cabal"]
    }

    static func title(on network: String) -> String {
        "Send USDC on \(network)"
    }

    static func headTitle(on network: String) -> String {
        "USDC on \(network)"
    }

    static func warning(on network: String) -> String {
        "Send only USDC on \(network). Transfers on other networks can't be recovered."
    }

    private var arrivingMicros: Int64 {
        if case .loaded(let balance) = state { return balance.arrivingMicros }
        return 0
    }

    var body: some View {
        Group {
            if let receivedMicros {
                received(receivedMicros)
            } else {
                entry
            }
        }
        .navigationTitle("Receive USDC")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var entry: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                ForEach(Self.order, id: \.self) { section in
                    switch section {
                    case .head:
                        head
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
                    case .notices:
                        notices
                            .padding(.horizontal, MonacoTheme.Space.gutter)
                    case .howItWorks:
                        howItWorks
                    }
                }
            }
            .padding(.top, MonacoTheme.Space.s)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        .monacoCanvas()
    }

    private func received(_ micros: Int64) -> some View {
        MoneyOutcome(
            tone: .done, title: "Deposit received",
            message: "\(UsdAmountFormatter.format(micros: micros)) is now available in your account.",
            rows: [
                MoneyKeyRow(label: "Asset", value: .words("USDC")),
                MoneyKeyRow(label: "Network", value: .words(network.displayName)),
                MoneyKeyRow(label: "Destination", value: .words("Cash")),
                MoneyKeyRow(label: "Amount", value: .data(UsdAmountFormatter.format(micros: micros))),
            ],
            identifier: "deposit-received"
        ) {
            Button("Fund a cabal", action: onFundCabal)
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("deposit-received-fund-button")
            Button("Back to Home", action: onHome)
                .buttonStyle(.monacoOutline)
                .accessibilityIdentifier("deposit-received-home-button")
        }
    }

    private var head: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text(Self.headTitle(on: network.displayName))
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.ink)
                .frame(maxWidth: .infinity, alignment: .leading)
                .accessibilityAddTraits(.isHeader)
            StatusBadge("Deposit", tone: .positive)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
    }

    @ViewBuilder
    private var notices: some View {
        if case .failed = state {
            MonacoErrorRow(thing: "your balance", identifier: "deposit-balance-error", retry: onRetryBalance)
        }
        if arrivingMicros > 0 {
            MonacoNotice(.warn, message: Self.waitingNotice)
                .accessibilityIdentifier("platform-balance-arriving")
        } else {
            MonacoNotice(.info, message: Self.warning(on: network.displayName))
                .accessibilityIdentifier("deposit-network-warning")
        }
        if let arrival = DepositAddressCard.arrivalNote(for: network) {
            MonacoNotice(.info, message: arrival)
                .accessibilityIdentifier("deposit-arrival-note")
        }
        if let fee = DepositAddressCard.feeNote(for: network) {
            MonacoNotice(.info, message: fee)
                .accessibilityIdentifier("deposit-fee-note")
        }
    }

    private var howItWorks: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            MonacoSectionHeader("How it works")
            ForEach(Array(Self.steps(on: network.displayName).enumerated()), id: \.offset) { index, step in
                stepView(number: index + 1, label: step)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("deposit-how-it-works")
    }

    private func stepView(number: Int, label: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.sm) {
            Text("\(number)")
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.brand)
                .frame(width: 26, height: 26)
                .background(Circle().fill(MonacoTheme.brandSoft))
                .accessibilityHidden(true)
            Text(label)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
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

    static func feeNote(for network: DepositNetwork) -> String? {
        guard !network.isSolana, let fee = network.estimatedFeeMicros, fee >= 5000 else { return nil }
        return "Fee about \(UsdAmountFormatter.format(micros: fee)). It comes out of what you send."
    }

    var body: some View {
        MonacoCard {
            VStack(spacing: MonacoTheme.Space.m) {
                Text("Your deposit address")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .frame(maxWidth: .infinity)
                switch content {
                case .loading:
                    loading
                case .ready(let address):
                    ready(address)
                case .unavailable:
                    EmptyView()
                }
            }
        }
    }

    private func ready(_ address: String) -> some View {
        VStack(spacing: MonacoTheme.Space.m) {
            DepositQRCode(address: address, showsSolanaMark: network.isSolana, onCopy: onCopy)
                .padding(.vertical, MonacoTheme.Space.m)
                .frame(maxWidth: .infinity)
                .background(
                    RoundedRectangle(cornerRadius: MonacoTheme.Radius.cardTight, style: .continuous)
                        .fill(MonacoTheme.surfaceSunken))

            MonacoWalletAddressText(address: address, textStyle: .footnote)
                .padding(.vertical, MonacoTheme.Space.sm)
                .padding(.horizontal, MonacoTheme.Space.sm)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(
                    RoundedRectangle(cornerRadius: MonacoTheme.Radius.field, style: .continuous)
                        .fill(MonacoTheme.surfaceSunken)
                )
                .accessibilityIdentifier(addressIdentifier)
                .onTapGesture {
                    onCopy(address)
                }

            Button("Copy address") {
                onCopy(address)
            }
            .buttonStyle(.monacoPrimary)
            .accessibilityIdentifier(copyIdentifier)
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
