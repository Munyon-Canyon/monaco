import MonacoAPI
import Observation

@Observable
@MainActor
public final class ChatSeenModel {
    public typealias Member = Components.Schemas.ChatSeenMember

    public private(set) var state: LoadState<[Member]> = .idle
    private let cabalID: String
    private let messageID: String
    private let api: APIClient
    private var generation = 0

    public init(cabalID: String, messageID: String, api: APIClient) {
        self.cabalID = cabalID
        self.messageID = messageID
        self.api = api
    }

    public func load() async {
        generation += 1
        let issued = generation
        state = .loading
        do {
            let (cabalID, messageID) = (cabalID, messageID)
            let seen = try await api.read { client in
                try await client.getChatSeenBy(
                    path: .init(id: cabalID), query: .init(messageId: messageID)
                ).ok.body.json
            }
            guard issued == generation else { return }
            state = .loaded(seen.members)
        } catch {
            guard issued == generation else { return }
            state = .failed(APIError(error))
        }
    }
}
