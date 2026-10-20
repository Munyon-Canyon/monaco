import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class PublicActivityModel {
    public enum Phase: Equatable, Sendable {
        case idle
        case loading
        case loaded
        case failed
    }

    public static let kinds = [FeedKind.proposal, .reply]
    public static let pageSize = 5

    public let userID: String
    public private(set) var phase: Phase = .idle
    public private(set) var items: [Components.Schemas.FeedItem] = []

    private let api: APIClient
    private var generation = 0

    public init(userID: String, api: APIClient) {
        self.userID = userID
        self.api = api
    }

    public func load() async {
        generation += 1
        let mine = generation
        if items.isEmpty { phase = .loading }
        do {
            let page = try await api.read { [userID] client in
                try await client.getFeed(query: Self.query(userID: userID)).ok.body.json
            }
            guard mine == generation else { return }
            items = page.items
            phase = .loaded
        } catch {
            guard mine == generation else { return }
            if items.isEmpty { phase = .failed }
        }
    }

    static func query(userID: String) -> Operations.GetFeed.Input.Query {
        .init(kind: kinds.map(\.rawValue).joined(separator: ","), actorId: userID, limit: pageSize)
    }
}
