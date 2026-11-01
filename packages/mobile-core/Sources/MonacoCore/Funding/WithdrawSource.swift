import Foundation
import MonacoAPI

public typealias WithdrawalStatus = Components.Schemas.WithdrawalStatus

public struct Withdrawal: Equatable, Sendable {
    public let withdrawalID: String
    public let status: WithdrawalStatus
    public let amountMicros: Int64
    public let txSignature: String?
    public let failCode: String?

    public init(
        withdrawalID: String, status: WithdrawalStatus, amountMicros: Int64, txSignature: String?,
        failCode: String? = nil
    ) {
        self.withdrawalID = withdrawalID
        self.status = status
        self.amountMicros = amountMicros
        self.txSignature = txSignature
        self.failCode = failCode
    }

    public var solscanURL: URL? {
        guard let txSignature, !txSignature.isEmpty else { return nil }
        return URL(string: "https://solscan.io/tx/\(txSignature)")
    }
}

public enum WithdrawFeeMode: Equatable, Sendable {
    case included, onTop
}

public struct WithdrawQuote: Equatable, Sendable {
    public let availableMicros: Int64
    public let feeMicros: Int64?
    public let feeKnown: Bool
    public let feeMode: WithdrawFeeMode
    public let maxAmountMicros: Int64
    public let receiveMicros: Int64?
    public let overMicros: Int64?
    public let belowMin: Bool
    public let minMicros: Int64
    public let arrivalSeconds: Int
    public let expiresAt: Date?

    public init(
        availableMicros: Int64, feeMicros: Int64?, feeKnown: Bool, feeMode: WithdrawFeeMode, maxAmountMicros: Int64,
        receiveMicros: Int64?, overMicros: Int64?, belowMin: Bool, minMicros: Int64, arrivalSeconds: Int,
        expiresAt: Date?
    ) {
        self.availableMicros = availableMicros
        self.feeMicros = feeMicros
        self.feeKnown = feeKnown
        self.feeMode = feeMode
        self.maxAmountMicros = maxAmountMicros
        self.receiveMicros = receiveMicros
        self.overMicros = overMicros
        self.belowMin = belowMin
        self.minMicros = minMicros
        self.arrivalSeconds = arrivalSeconds
        self.expiresAt = expiresAt
    }
}

public struct WithdrawSource: Sendable {
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func withdraw(
        micros: Int64, toAddress: String, network: WithdrawNetwork = .solana, submission: IdempotentSubmission
    ) async throws -> Withdrawal {
        let body = network.request(micros: micros, toAddress: toAddress)
        let accepted = try await api.submit(submission, payload: body, operation: "withdraw") { client, key in
            let authorization = try await client.authorizeWithdrawal(body: .json(body)).created.body.json
            return try await client.withdraw(
                headers: .init(idempotencyKey: key, withdrawalAuthorization: authorization.authorizationId),
                body: .json(body)
            ).accepted.body.json
        }
        return Withdrawal(
            withdrawalID: accepted.withdrawalId, status: accepted.status, amountMicros: micros,
            txSignature: accepted.txSignature)
    }

    public func quote(
        network: WithdrawNetwork, amountMicros: Int64?, toAddress: String?
    ) async throws -> WithdrawQuote {
        let body = Components.Schemas.WithdrawPreviewRequest(
            destinationKind: network.isEVM ? .evm : nil,
            network: network.isEVM ? .init(rawValue: network.rawValue) : nil,
            amountMicros: amountMicros.map(String.init), toAddress: toAddress)
        let wire = try await api.read { try await $0.previewWithdrawal(body: .json(body)).ok.body.json }
        let receive = try AccountBalance.micros(wire.receiveMicros)
        return WithdrawQuote(
            availableMicros: try AccountBalance.micros(wire.availableMicros),
            feeMicros: try wire.feeMicros.map(AccountBalance.micros), feeKnown: wire.feeKnown,
            feeMode: wire.feeMode == .included ? .included : .onTop,
            maxAmountMicros: try AccountBalance.micros(wire.maxAmountMicros),
            receiveMicros: amountMicros == nil || (network.isEVM && toAddress == nil) ? nil : receive,
            overMicros: try wire.overMicros.map(AccountBalance.micros),
            belowMin: wire.belowMin, minMicros: try AccountBalance.micros(wire.minMicros),
            arrivalSeconds: wire.arrivalSeconds, expiresAt: wire.expiresAt)
    }

    public func withdrawal(id: String) async throws -> Withdrawal {
        let wire = try await api.read { try await $0.getMyWithdrawal(path: .init(id: id)).ok.body.json }
        return Withdrawal(
            withdrawalID: wire.withdrawalId, status: wire.status,
            amountMicros: try AccountBalance.micros(wire.amountMicros), txSignature: wire.txSignature,
            failCode: wire.failCode)
    }
}

#if DEBUG
extension WithdrawModel {
    public static func sample(
        transport: some ClientTransport, ownDepositAddress: String?,
        clock: any Clock<Duration>
    ) -> WithdrawModel {
        let api = APIClient(
            serverURL: URL(filePath: "/sample"), tokens: FlowScenarioTokens(), transport: transport)
        let hints = FlowScenarioHints()
        let settlements = SettlementWatcher(api: api, hints: hints, journal: PendingIntentJournal())
        settlements.bind(accountID: "sample")
        return WithdrawModel(
            api: api, hints: hints, submission: IdempotentSubmission(), settlements: settlements,
            ownDepositAddress: ownDepositAddress, clock: clock,
            balance: BalanceSource(api: api, hints: hints))
    }
}
#endif
