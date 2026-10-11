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

public struct WithdrawPreview: Equatable, Sendable {
    public let receiveMicros: Int64
    public let expiresAt: Date?

    public init(receiveMicros: Int64, expiresAt: Date?) {
        self.receiveMicros = receiveMicros
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
            try await client.withdraw(headers: .init(idempotencyKey: key), body: .json(body)).accepted.body.json
        }
        return Withdrawal(
            withdrawalID: accepted.withdrawalId, status: accepted.status, amountMicros: micros,
            txSignature: accepted.txSignature)
    }

    public func preview(micros: Int64, toAddress: String, network: WithdrawNetwork) async throws -> WithdrawPreview {
        let body = network.request(micros: micros, toAddress: toAddress)
        let preview = try await api.read { try await $0.previewWithdrawal(body: .json(body)).ok.body.json }
        return WithdrawPreview(
            receiveMicros: try AccountBalance.micros(preview.receiveMicros), expiresAt: preview.expiresAt)
    }

    public func withdrawal(id: String) async throws -> Withdrawal {
        let wire = try await api.read { try await $0.getMyWithdrawal(path: .init(id: id)).ok.body.json }
        return Withdrawal(
            withdrawalID: wire.withdrawalId, status: wire.status,
            amountMicros: try AccountBalance.micros(wire.amountMicros), txSignature: wire.txSignature,
            failCode: wire.failCode)
    }
}
