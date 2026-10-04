import Foundation

struct JoinRequestDTO: Codable, Equatable, Identifiable {
    let id: String
    let userId: String
    let displayName: String
    var profilePhotoUrl: String? = nil
    let requestedAt: String
}

struct JoinRequestsListResponse: Codable, Equatable {
    let items: [JoinRequestDTO]
}
