import MonacoAPI
import Observation

public struct HomeInviteRow: Equatable, Sendable, Identifiable {
    public let invite: ReceivedCabalInvite
    public let invitedByName: String

    public var id: String { invite.id }
    public var name: String { CabalHeaderCopy.shortName(invite.cabalName) }
    public var subtitle: String { "Invited by \(invitedByName) · \(invite.members)" }

    init(_ invite: Components.Schemas.CabalInvite) {
        self.invite = ReceivedCabalInvite(invite)
        let inviter = invite.invitedBy
        if !inviter.displayName.isEmpty {
            invitedByName = inviter.displayName
        } else if let handle = inviter.handle {
            invitedByName = "@\(handle)"
        } else {
            invitedByName = "Someone"
        }
    }
}

@Observable
@MainActor
public final class HomeCabalContextModel {
    public private(set) var invites: [HomeInviteRow]?

    private let api: APIClient
    private var invitesRead = false

    public init(api: APIClient) {
        self.api = api
    }

    public func load(refreshing: Bool = false) async {
        guard refreshing || !invitesRead else { return }
        let fetched = try? await api.read { try await $0.getMyCabalInvites().ok.body.json }
        guard !Task.isCancelled else { return }
        if let fetched { invites = fetched.map(HomeInviteRow.init) }
        invitesRead = true
    }

    public func observe(hints: any HintSource) async {
        for await _ in hints.hints(matching: .user(what: "cabal_invites")) {
            await load(refreshing: true)
        }
    }
}
