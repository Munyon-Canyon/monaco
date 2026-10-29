import Foundation
import MonacoAPI
import MonacoFunding
import Observation

@Observable
@MainActor
public final class WithdrawModel {
    public enum State: Equatable, Sendable {
        case empty
        case typed
        case invalid(maxMicros: Int64, overMicros: Int64)
        case belowMinimum(minMicros: Int64)
        case needsAddress
        case submitting
        case settled(
            amountMicros: Int64, to: String, network: WithdrawNetwork, arrivalSeconds: Int, cashAfterMicros: Int64?)
        case failed(String)
    }

    public private(set) var address = ""
    public private(set) var network: WithdrawNetwork = .solana
    public private(set) var amountText = ""
    public private(set) var quoteFailed = false
    public private(set) var isQuoting = false

    private var outcome: Outcome?
    private var quoted: (key: QuoteKey, quote: WithdrawQuote)?
    private var refusedAddress: String?

    private enum Outcome {
        case submitting
        case settled(micros: Int64, to: String, network: WithdrawNetwork, arrival: Int, cashAfter: Int64?)
        case failed(String)
    }

    private struct QuoteKey: Equatable {
        let network: WithdrawNetwork
        let amountMicros: Int64?
        let address: String?
    }

    private let source: WithdrawSource
    private let withdrawing: Withdrawing
    private let settlements: SettlementWatcher
    private let balance: BalanceSource?
    private let ownDepositAddress: String?
    private let demoMode: Bool
    private let clock: any Clock<Duration>
    @ObservationIgnored private var quoteTask: Task<Void, Never>?
    @ObservationIgnored private var requestedKey: QuoteKey?
    private var withdrawAll = false

    static let debounce: Duration = .milliseconds(250)
    static let retryDelay: Duration = .seconds(2)

    public init(
        api: APIClient, hints: any HintSource, submission: IdempotentSubmission, settlements: SettlementWatcher,
        ownDepositAddress: String?, demoMode: Bool, clock: any Clock<Duration>, balance: BalanceSource? = nil
    ) {
        source = WithdrawSource(api: api)
        withdrawing = Withdrawing(source: source, hints: hints, submission: submission)
        self.settlements = settlements
        self.balance = balance
        self.ownDepositAddress = ownDepositAddress
        self.demoMode = demoMode
        self.clock = clock
    }

    public var networks: [WithdrawNetwork] {
        let base: [WithdrawNetwork] = [.solana, .base, .arbitrum, .ethereum]
        return demoMode ? base + [.monad] : base
    }

    public var amountMicros: Int64 { AmountText.micros(amountText) ?? 0 }

    public var quote: WithdrawQuote? {
        guard let quoted, quoted.key.network == network else { return nil }
        return quoted.quote
    }

    public var maxMicros: Int64? { quote?.maxAmountMicros }

    public var validAddress: String? {
        let trimmed = address.trimmingCharacters(in: .whitespacesAndNewlines)
        guard case .success(let valid) = network.validate(trimmed, ownDepositAddress: ownDepositAddress) else {
            return nil
        }
        return valid
    }

    public var addressProblem: String? {
        if let refusedAddress { return refusedAddress }
        let trimmed = address.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty,
            case .failure(let problem) = network.validate(trimmed, ownDepositAddress: ownDepositAddress)
        else { return nil }
        return WithdrawCopy.addressMessage(for: problem, network: network)
    }

    private var currentKey: QuoteKey {
        QuoteKey(
            network: network, amountMicros: amountMicros > 0 ? amountMicros : nil, address: validAddress)
    }

    private var freshQuote: WithdrawQuote? {
        guard let quoted, quoted.key == currentKey else { return nil }
        return quoted.quote
    }

    public var state: State {
        switch outcome {
        case .submitting: return .submitting
        case .settled(let micros, let to, let network, let arrival, let cashAfter):
            return .settled(
                amountMicros: micros, to: to, network: network, arrivalSeconds: arrival, cashAfterMicros: cashAfter)
        case .failed(let message): return .failed(message)
        case nil: break
        }
        guard amountMicros > 0 else { return .empty }
        if let fresh = freshQuote {
            if fresh.belowMin { return .belowMinimum(minMicros: fresh.minMicros) }
            if let over = fresh.overMicros, over > 0 {
                return .invalid(maxMicros: fresh.maxAmountMicros, overMicros: over)
            }
        }
        return validAddress == nil ? .needsAddress : .typed
    }

    public var canSubmit: Bool {
        guard validAddress != nil, !quoteFailed else { return false }
        switch state {
        case .typed, .failed: return freshQuote != nil
        default: return false
        }
    }

    public var line: String? {
        switch state {
        case .empty:
            if quoteFailed { return WithdrawCopy.feeFailed }
            return WithdrawCopy.emptyLine(feeMicros: quote?.feeMicros, network: network)
        case .typed, .needsAddress, .failed, .invalid:
            if quoteFailed { return WithdrawCopy.feeFailed }
            guard let fresh = freshQuote, let receive = fresh.receiveMicros else { return nil }
            return WithdrawCopy.typedLine(receiveMicros: receive, feeMicros: fresh.feeMicros)
        case .belowMinimum(let min): return WithdrawCopy.belowMinimum(minMicros: min)
        case .settled(_, _, let network, let arrival, _):
            return WithdrawCopy.settledLine(network: network, arrivalSeconds: arrival)
        case .submitting: return nil
        }
    }

    public var limitSlot: String? {
        guard case .invalid(let max, _) = state else { return nil }
        return WithdrawCopy.limitSlot(maxMicros: max)
    }

    public var checkLine: String? {
        guard case .settled(let micros, let to, _, _, _) = state else { return nil }
        return WithdrawCopy.settledCheck(amountMicros: micros, address: to)
    }

    public var buttonTitle: String? {
        switch state {
        case .typed, .failed, .submitting: WithdrawCopy.button(amountMicros: amountMicros)
        case .needsAddress: WithdrawCopy.addAddress
        case .empty, .invalid, .belowMinimum, .settled: nil
        }
    }

    public func load() {
        scheduleQuote()
    }

    public func setAmountText(_ text: String) {
        guard text != amountText else { return }
        amountText = text
        withdrawAll = false
        clearFailure()
        scheduleQuote()
    }

    public func useMax() {
        guard let max = maxMicros else { return }
        amountText = AmountText.plain(AmountText.roundDownToCents(AmountText.dollars(micros: max)))
        withdrawAll = true
        clearFailure()
        scheduleQuote()
    }

    public func setAddress(_ text: String) {
        guard text != address else { return }
        address = text
        refusedAddress = nil
        clearFailure()
        scheduleQuote()
    }

    public func setNetwork(_ next: WithdrawNetwork) {
        guard next != network, networks.contains(next) else { return }
        network = next
        refusedAddress = nil
        clearFailure()
        scheduleQuote()
    }

    public func retryQuote() {
        requestedKey = nil
        scheduleQuote()
    }

    private func clearFailure() {
        if case .failed = outcome { outcome = nil }
    }

    private func scheduleQuote() {
        let key = currentKey
        guard key != requestedKey else { return }
        requestedKey = key
        quoteFailed = false
        quoteTask?.cancel()
        isQuoting = true
        quoteTask = Task { [clock, source] in
            do {
                try await clock.sleep(for: Self.debounce)
                let quote: WithdrawQuote
                do {
                    quote = try await source.quote(
                        network: key.network, amountMicros: key.amountMicros, toAddress: key.address)
                } catch is CancellationError {
                    return
                } catch {
                    try await clock.sleep(for: Self.retryDelay)
                    quote = try await source.quote(
                        network: key.network, amountMicros: key.amountMicros, toAddress: key.address)
                }
                guard !Task.isCancelled else { return }
                self.quoted = (key, quote)
                self.quoteFailed = false
                self.isQuoting = false
            } catch is CancellationError {
            } catch {
                guard !Task.isCancelled else { return }
                self.quoteFailed = true
                self.isQuoting = false
            }
        }
    }

    public func submit() async {
        guard canSubmit, let to = validAddress, let quote = freshQuote else { return }
        let micros = amountMicros
        let arrival = quote.arrivalSeconds
        let sentNetwork = network
        let fullBalanceLabel = withdrawAll ? WithdrawCopy.fullBalanceLabel(micros: micros) : nil
        outcome = .submitting
        switch await withdrawing.submit(micros: micros, toAddress: to, network: sentNetwork) {
        case .accepted(let accepted):
            if case .failed = withdrawing.progress {
                outcome = .failed(WithdrawProgress.didNotGoThrough)
                return
            }
            outcome = .settled(
                micros: accepted.amountMicros, to: to, network: sentNetwork, arrival: arrival, cashAfter: nil)
            settlements.watch(
                SettlementTicket(
                    kind: .withdraw, resourceID: accepted.withdrawalID, fullBalanceLabel: fullBalanceLabel))
            await reloadBalance(micros: accepted.amountMicros, to: to, network: sentNetwork, arrival: arrival)
        case .refused(.address(let message)):
            outcome = nil
            refusedAddress =
                message == WithdrawFailure.ownWallet ? WithdrawCopy.ownDeposit : WithdrawCopy.invalidAddress
        case .refused(.toast(let message)):
            outcome = .failed(message)
        case .unconfirmed(let message):
            outcome = .failed(message)
        }
    }

    private func reloadBalance(micros: Int64, to: String, network: WithdrawNetwork, arrival: Int) async {
        guard let balance else { return }
        await balance.load()
        guard case .settled = outcome, let cashAfter = balance.balance?.availableMicros else { return }
        outcome = .settled(micros: micros, to: to, network: network, arrival: arrival, cashAfter: cashAfter)
    }
}
