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
        case processing(amountMicros: Int64, to: String, network: WithdrawNetwork, arrivalSeconds: Int)
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
        case accepted(Accepted)
        case failed(String)
    }

    private struct Accepted {
        let id: String
        let micros: Int64
        let to: String
        let network: WithdrawNetwork
        let arrival: Int
        let confirmedAtOnce: Bool
        var cashAfter: Int64?
    }

    private struct QuoteKey: Equatable {
        let network: WithdrawNetwork
        let amountMicros: Int64?
        let address: String?
    }

    private let source: WithdrawSource
    private let hints: any HintSource
    private let submission: IdempotentSubmission
    private var withdrawing: Withdrawing
    private let settlements: SettlementWatcher
    private let balance: BalanceSource?
    private let ownDepositAddress: String?
    private let clock: any Clock<Duration>
    private var presentedID: String?
    private var closed = false
    @ObservationIgnored private var quoteTask: Task<Void, Never>?
    @ObservationIgnored private var requestedKey: QuoteKey?
    private var withdrawAll = false

    static let debounce: Duration = .milliseconds(250)
    static let retryDelay: Duration = .seconds(2)

    public init(
        api: APIClient, hints: any HintSource, submission: IdempotentSubmission, settlements: SettlementWatcher,
        ownDepositAddress: String?, clock: any Clock<Duration>, balance: BalanceSource? = nil
    ) {
        let source = WithdrawSource(api: api)
        self.source = source
        self.hints = hints
        self.submission = submission
        withdrawing = Withdrawing(source: source, hints: hints, submission: submission)
        self.settlements = settlements
        self.balance = balance
        self.ownDepositAddress = ownDepositAddress
        self.clock = clock
    }

    public var networks: [WithdrawNetwork] {
        WithdrawNetwork.allCases.filter { LaunchFeatures.offersMoney($0.rawValue) }
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
        if suggestedNetwork != nil { return WithdrawCopy.baseAddressOnSolana }
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
        case .accepted(let accepted): return state(of: accepted)
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

    private func state(of accepted: Accepted) -> State {
        let settled = State.settled(
            amountMicros: accepted.micros, to: accepted.to, network: accepted.network,
            arrivalSeconds: accepted.arrival, cashAfterMicros: accepted.cashAfter)
        if accepted.confirmedAtOnce { return settled }
        switch settlements.withdrawals[accepted.id] {
        case .confirmed: return settled
        case .failed: return .failed(WithdrawProgress.didNotGoThrough)
        case .idle, .submitting, .submitted, nil:
            return .processing(
                amountMicros: accepted.micros, to: accepted.to, network: accepted.network,
                arrivalSeconds: accepted.arrival)
        }
    }

    public var canSubmit: Bool {
        guard validAddress != nil, refusedAddress == nil, !quoteFailed else { return false }
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
            guard let fresh = freshQuote else { return nil }
            guard let receive = (fresh.overMicros ?? 0) > 0 ? amountMicros : fresh.receiveMicros else {
                return WithdrawCopy.emptyLine(feeMicros: fresh.feeMicros, network: network)
            }
            return WithdrawCopy.typedLine(receiveMicros: receive, feeMicros: fresh.feeMicros, isEstimate: network.isEVM)
        case .belowMinimum(let min): return WithdrawCopy.belowMinimum(minMicros: min)
        case .settled(_, _, let network, let arrival, _):
            return WithdrawCopy.settledLine(network: network, arrivalSeconds: arrival)
        case .processing(_, _, let network, let arrival):
            return WithdrawCopy.arrivalNote(network: network, arrivalSeconds: arrival)
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
        case .processing: WithdrawCopy.processing
        case .needsAddress: WithdrawCopy.addAddress
        case .empty, .invalid, .belowMinimum, .settled: nil
        }
    }

    public func load() {
        scheduleQuote()
    }

    private var isLocked: Bool {
        switch state {
        case .submitting, .processing, .settled: true
        default: false
        }
    }

    public func setAmountText(_ text: String) {
        guard !isLocked, text != amountText else { return }
        amountText = text
        withdrawAll = false
        clearFailure()
        scheduleQuote()
    }

    public func useMax() {
        guard !isLocked, let max = maxMicros else { return }
        amountText = AmountText.plain(AmountText.roundDownToCents(AmountText.dollars(micros: max)))
        withdrawAll = true
        clearFailure()
        scheduleQuote()
    }

    public func setAddress(_ text: String) {
        guard !isLocked, text != address else { return }
        address = text
        refusedAddress = nil
        clearFailure()
        scheduleQuote()
    }

    public func setNetwork(_ next: WithdrawNetwork) {
        guard !isLocked, next != network, networks.contains(next) else { return }
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
        if case .failed = outcome {
            outcome = nil
        } else if case .accepted = outcome, case .failed = state {
            release()
            outcome = nil
        }
    }

    public func dismiss() {
        closed = true
        release()
    }

    private func release() {
        guard let id = presentedID else { return }
        presentedID = nil
        settlements.dismiss(id)
    }
}

extension WithdrawModel {
    public var suggestedNetwork: WithdrawNetwork? {
        guard network == .solana, networks.contains(.base),
            case .success = EVMAddress.validate(address.trimmingCharacters(in: .whitespacesAndNewlines))
        else { return nil }
        return .base
    }

    public func switchToSuggestedNetwork() {
        guard let suggestedNetwork else { return }
        setNetwork(suggestedNetwork)
    }

    public var buttonIsEnabled: Bool {
        switch state {
        case .typed, .failed: canSubmit || quoteFailed
        default: true
        }
    }

    public func observe() async {
        for await _ in hints.hints(matching: .user(what: BalanceSource.refreshingHint)) {
            guard !closed, !isLocked else { continue }
            retryQuote()
        }
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
        release()
        if withdrawing.progress != .idle {
            withdrawing = Withdrawing(source: source, hints: hints, submission: submission)
        }
        outcome = .submitting
        switch await withdrawing.submit(micros: micros, toAddress: to, network: sentNetwork) {
        case .accepted(let accepted):
            if case .failed = withdrawing.progress {
                outcome = .failed(WithdrawProgress.didNotGoThrough)
                return
            }
            var confirmedAtOnce = false
            if case .confirmed = withdrawing.progress { confirmedAtOnce = true }
            outcome = .accepted(
                Accepted(
                    id: accepted.withdrawalID, micros: accepted.amountMicros, to: to, network: sentNetwork,
                    arrival: arrival, confirmedAtOnce: confirmedAtOnce, cashAfter: nil))
            if !confirmedAtOnce {
                settlements.watch(
                    SettlementTicket(
                        kind: .withdraw, resourceID: accepted.withdrawalID, fullBalanceLabel: fullBalanceLabel))
                if !closed {
                    settlements.present(accepted.withdrawalID)
                    presentedID = accepted.withdrawalID
                }
            }
            await reloadBalance()
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

    private func reloadBalance() async {
        guard let balance else { return }
        await balance.load()
        guard case .accepted(var accepted) = outcome, let cashAfter = balance.balance?.availableMicros else { return }
        accepted.cashAfter = cashAfter
        outcome = .accepted(accepted)
    }
}
