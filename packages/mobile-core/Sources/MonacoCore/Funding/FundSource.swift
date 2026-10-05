import Foundation
import MonacoAPI

public typealias FundTransferStatus = Components.Schemas.FundTransferStatus

public struct FundTransfer: Equatable, Sendable {
    public let transferID: String
    public let status: FundTransferStatus
    public let amountMicros: Int64
    public let shareUnits: String?
    public let failCode: String?

    public init(
        transferID: String, status: FundTransferStatus, amountMicros: Int64, shareUnits: String? = nil,
        failCode: String? = nil
    ) {
        self.transferID = transferID
        self.status = status
        self.amountMicros = amountMicros
        self.shareUnits = shareUnits
        self.failCode = failCode
    }
}

public struct FundSource: Sendable {
    private let api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    public func fund(cabalID: String, micros: Int64, submission: IdempotentSubmission) async throws -> FundTransfer {
        let body = Components.Schemas.FundRequest(amountMicros: String(micros))
        let accepted = try await api.submit(submission, payload: body, operation: "fundCabal") { client, key in
            try await client.fundCabal(
                path: .init(id: cabalID), headers: .init(idempotencyKey: key), body: .json(body)
            ).accepted.body.json
        }
        return FundTransfer(transferID: accepted.transferId, status: accepted.status, amountMicros: micros)
    }

    public func transfer(id: String) async throws -> FundTransfer {
        let wire = try await api.read { try await $0.getFundTransfer(path: .init(id: id)).ok.body.json }
        return FundTransfer(
            transferID: id, status: wire.status, amountMicros: try AccountBalance.micros(wire.amountMicros),
            shareUnits: wire.shareUnits, failCode: wire.failCode)
    }
}
