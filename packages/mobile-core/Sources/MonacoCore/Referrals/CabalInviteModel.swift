import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalInviteModel {
    public static let hints = ["proposal_created", "proposal_updated", "members"]

    public private(set) var state: LoadState<CabalInvitePreview> = .idle

    public let cabalID: String
    private let api: APIClient
    private let hints: any HintSource
    private let inviteLinks: InviteLinkModel
    private let now: @Sendable () -> Date
    private var generation = 0

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(
        api: APIClient, hints: any HintSource, cabalID: String, inviteLinks: InviteLinkModel,
        now: @escaping @Sendable () -> Date = WallClock.now
    ) {
        self.api = api
        self.hints = hints
        self.cabalID = cabalID
        self.inviteLinks = inviteLinks
        self.now = now
    }

    public var preview: CabalInvitePreview? {
        if case .loaded(let preview) = state { return preview }
        return nil
    }

    public var link: URL? { inviteLinks.cabalLink(cabalID: cabalID) }

    public func load() async {
        generation += 1
        let current = generation
        if preview == nil { state = .loading }
        if inviteLinks.code == nil { await inviteLinks.load() }
        guard current == generation, !Task.isCancelled else { return }
        guard let code = inviteLinks.code else {
            if preview == nil { state = .failed(Self.linkError(inviteLinks.state)) }
            return
        }
        let result = await fetch(code: code.value)
        guard current == generation, !Task.isCancelled else { return }
        switch result {
        case .success(let loaded):
            state = .loaded(loaded)
        case .failure(let error):
            if preview == nil { state = .failed(error) }
        }
    }

    public func observe() async {
        var streams = Self.hints.map { hints.hints(matching: .cabal(id: cabalID, what: $0)) }
        streams.append(hints.hints(matching: .global(what: "feed")))
        await refresher.observe(streams)
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private func fetch(code: String) async -> Result<CabalInvitePreview, APIError> {
        do {
            let cabalID = cabalID
            let response = try await api.read { client in
                try await client.getReferralCabal(path: .init(code: code, cabalId: cabalID)).ok.body.json
            }
            guard let preview = CabalInviteCopy.preview(response, now: now()) else {
                return .failure(.decoding("the invite preview did not resolve for cabal \(cabalID)"))
            }
            return .success(preview)
        } catch {
            return .failure(APIError(error))
        }
    }

    private static func linkError(_ state: InviteLinkState) -> APIError {
        if case .failed(let error) = state { return error }
        return .decoding("the invite link is not ready")
    }
}
