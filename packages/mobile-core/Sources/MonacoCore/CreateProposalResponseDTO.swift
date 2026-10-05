import Foundation

public struct CreateProposalResponseDTO: Codable, Equatable, Sendable {
    public let proposalId: String

    public init(proposalId: String) {
        self.proposalId = proposalId
    }
}
