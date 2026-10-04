import Foundation
import MonacoAPI
import Observation

public struct AccountBalance: Equatable, Sendable {
    public let availableMicros: Int64
    public let onChainMicros: Int64
    public let inFlightMicros: Int64
    public let depositAddress: String
    public let asOf: Date

    public init(
        availableMicros: Int64, onChainMicros: Int64, inFlightMicros: Int64, depositAddress: String, asOf: Date
    ) {
        self.availableMicros = availableMicros
        self.onChainMicros = onChainMicros
        self.inFlightMicros = inFlightMicros
        self.depositAddress = depositAddress
        self.asOf = asOf
    }

    public init(_ balance: Components.Schemas.Balance) throws {
        self.init(
            availableMicros: try Self.micros(balance.availableMicros),
            onChainMicros: try Self.micros(balance.onChainMicros),
            inFlightMicros: try Self.micros(balance.inFlightMicros),
            depositAddress: balance.depositAddress,
            asOf: balance.asOf
        )
    }

    public static func micros(_ wire: String) throws -> Int64 {
        let digits = UInt8(ascii: "0")...UInt8(ascii: "9")
        guard !wire.isEmpty, wire.utf8.allSatisfy(digits.contains), let value = Int64(wire) else {
            throw APIError.decoding("an amount in micros: \(wire)")
        }
        return value
    }
}

public enum BalanceChange: Equatable, Sendable {
    case deposited(Int64)

    public var message: String {
        switch self {
        case .deposited(let micros): "Deposit received: \(UsdAmountFormatter.format(micros: micros))"
        }
    }

    public static func detect(previous: AccountBalance?, current: AccountBalance) -> BalanceChange? {
        guard let previous, current.inFlightMicros == previous.inFlightMicros,
            current.availableMicros > previous.availableMicros
        else { return nil }
        return .deposited(current.availableMicros - previous.availableMicros)
    }
}

@Observable
@MainActor
public final class BalanceSource {
    public static let refreshingHint = "balance_changed"

    public private(set) var state: LoadState<AccountBalance> = .idle
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    private let api: APIClient
    private let hints: any HintSource
    private let refresher: HintRefresher
    private var generation = 0

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
        let hook = BalanceReloadHook()
        refresher = HintRefresher { await hook.run?() }
        hook.run = { [weak self] in
            await self?.read(quietly: true)
        }
    }

    public var balance: AccountBalance? {
        if case .loaded(let balance) = state { return balance }
        return nil
    }

    public func load() async {
        await read(quietly: false)
    }

    private func read(quietly: Bool) async {
        generation += 1
        let issued = generation
        let shown = state
        if balance == nil, !quietly { state = .loading }
        do {
            let wire = try await api.read { try await $0.getMyBalance().ok.body.json }
            let fresh = try AccountBalance(wire)
            guard issued == generation else { return }
            state = .loaded(fresh)
            lastError = nil
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            let error = APIError(error)
            if balance == nil { state = .failed(error) }
            if quietly, Self.alreadyOnScreen(shown) { return }
            lastError = error
            failureTick += 1
        }
    }

    private static func alreadyOnScreen(_ state: LoadState<AccountBalance>) -> Bool {
        switch state {
        case .loaded, .failed: true
        case .idle, .loading: false
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: Self.refreshingHint)))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public static func message(for error: APIError) -> String {
        if case .problem(let problem) = error, problem.code == .known(.rpcUnavailable) {
            return "Balance is temporarily unavailable. Pull to refresh."
        }
        return ToastCopy.message(for: error)
    }
}

@MainActor
private final class BalanceReloadHook {
    var run: (@MainActor () async -> Void)?
}
