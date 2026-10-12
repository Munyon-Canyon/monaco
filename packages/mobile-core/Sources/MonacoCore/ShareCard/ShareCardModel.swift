import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class ShareCardModel {
    public private(set) var state: LoadState<ShareCardContent> = .idle

    private let api: APIClient?
    private let locale: Locale
    private let timeZone: TimeZone
    private let sleep: @Sendable (Duration) async throws -> Void
    private var generation = 0

    public init(
        api: APIClient,
        locale: Locale = .current,
        timeZone: TimeZone = .current,
        sleep: @escaping @Sendable (Duration) async throws -> Void = { try await Task.sleep(for: $0) }
    ) {
        self.api = api
        self.locale = locale
        self.timeZone = timeZone
        self.sleep = sleep
    }

    private init(state: LoadState<ShareCardContent>) {
        api = nil
        locale = .current
        timeZone = .current
        sleep = { _ in }
        self.state = state
    }

    public static func preview(_ content: ShareCardContent) -> ShareCardModel {
        ShareCardModel(state: .loaded(content))
    }

    public var content: ShareCardContent? {
        if case .loaded(let content) = state { return content }
        return nil
    }

    public func loadCabal(
        cabalID: String, cabal: Components.Schemas.Cabal, pot: Components.Schemas.CabalPot? = nil
    ) async {
        await load { api, links in
            async let invite = links()
            async let history = api.read {
                try await $0.getCabalValueHistory(path: .init(id: cabalID), query: .init(range: .all)).ok.body.json
            }
            let pot =
                if let pot { pot } else {
                    try await api.read { try await $0.getCabalPot(path: .init(id: cabalID)).ok.body.json }
                }
            let curve = try await history
            let links = await invite
            guard
                let content = ShareCardContent.cabal(
                    cabal, pot: pot, history: curve, links: links, locale: self.locale, timeZone: self.timeZone)
            else { throw APIError.decoding("a closed cabal has no share card") }
            return content
        }
    }

    public func loadMe(profile: SessionProfile, portfolio: Components.Schemas.MyPortfolio? = nil) async {
        await load { api, links in
            async let invite = links()
            async let history = api.read { try await $0.getMyPnlHistory(query: .init(range: .all)).ok.body.json }
            let portfolio =
                if let portfolio { portfolio } else {
                    try await api.read { try await $0.getMyPortfolio().ok.body.json }
                }
            let curve = try await history
            let links = await invite
            return ShareCardContent.me(
                profile: profile, portfolio: portfolio, history: curve, links: links, locale: self.locale,
                timeZone: self.timeZone)
        }
    }

    private func load(
        _ build: (APIClient, @escaping @Sendable () async -> InviteLinks?) async throws -> ShareCardContent
    ) async {
        guard let api else { return }
        generation += 1
        let issued = generation
        state = .loading
        do {
            let sleep = sleep
            let content = try await build(api) { await Self.inviteLinks(api: api, sleep: sleep) }
            guard issued == generation, !Task.isCancelled else { return }
            state = .loaded(content)
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            state = .failed(APIError(error))
        }
    }

    private nonisolated static func inviteLinks(
        api: APIClient, sleep: @Sendable (Duration) async throws -> Void
    ) async -> InviteLinks? {
        for attempt in 0..<2 {
            do {
                let code = try await api.read { try await $0.getMyReferralCode().ok.body.json }
                return InviteLinks(code)
            } catch {
                guard attempt == 0, isPending(APIError(error)) else { return nil }
                do { try await sleep(InviteLinkModel.pendingRetryDelay) } catch { return nil }
            }
        }
        return nil
    }

    private nonisolated static func isPending(_ error: APIError) -> Bool {
        guard case .problem(let problem) = error, case .known(.referralCodePending) = problem.code else {
            return false
        }
        return true
    }
}
