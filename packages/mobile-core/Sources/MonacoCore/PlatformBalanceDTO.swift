import Foundation

public struct FundGroupRequestDTO: Encodable, Sendable {
    public let amount: Int64

    public init(amount: Int64) {
        self.amount = amount
    }
}

public struct FundGroupResponseDTO: Decodable, Equatable, Sendable {
    public let depositId: String
    public let groupId: String
    public let amount: Int64
    public let status: String
    public let fromAddress: String

    public init(depositId: String, groupId: String, amount: Int64, status: String, fromAddress: String) {
        self.depositId = depositId
        self.groupId = groupId
        self.amount = amount
        self.status = status
        self.fromAddress = fromAddress
    }
}
