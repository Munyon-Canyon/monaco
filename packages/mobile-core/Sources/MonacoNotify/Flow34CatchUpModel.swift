import Foundation
import MonacoAPI
import MonacoFlows
import Observation

public struct CatchUpScreen: Equatable, Sendable {
    public let count: Int
    public let cabals: [Components.Schemas.CatchUpCabal]
    public let waiting: [Components.Schemas.CatchUpItem]
    public let replies: [Components.Schemas.CatchUpItem]
    public let invites: [Components.Schemas.CatchUpItem]
    public let results: [Components.Schemas.CatchUpItem]
    public let cleared: [Components.Schemas.CatchUpItem]

    public var isCaughtUp: Bool { count == 0 }

    public init(_ read: Components.Schemas.CatchUp) {
        count = read.count
        cabals = read.cabals
        waiting = read.waiting
        replies = read.replies
        invites = read.invites
        results = read.results
        cleared = read.cleared
    }
}

@Observable
@MainActor
public final class CatchUpModel {
    public private(set) var state: LoadState<CatchUpScreen> = .idle

    private let api: APIClient
    private let hints: any HintSource
    private let clock: @Sendable () -> Date
    private let calendar: Calendar
    private var generation = 0
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(
        api: APIClient, hints: any HintSource, clock: @escaping @Sendable () -> Date, calendar: Calendar = .current
    ) {
        self.api = api
        self.hints = hints
        self.clock = clock
        self.calendar = calendar
    }

    public func load() async {
        generation += 1
        let mine = generation
        let since = calendar.startOfDay(for: clock())
        if !isLoaded { state = .loading }
        do {
            let read = try await api.read { client in
                try await client.getMyCatchUp(query: .init(since: since)).ok.body.json
            }
            guard mine == generation else { return }
            state = .loaded(CatchUpScreen(read))
        } catch {
            guard mine == generation, !isLoaded else { return }
            state = .failed(APIError(error))
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: "catch_up_changed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private var isLoaded: Bool {
        if case .loaded = state { return true }
        return false
    }
}

#if DEBUG
extension CatchUpModel {
    public static func preview(state: LoadState<CatchUpScreen>, now: Date) -> CatchUpModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        let model = CatchUpModel(
            api: APIClient(serverURL: serverURL, tokens: PreviewTokens()),
            hints: PreviewHints(), clock: { now })
        model.state = state
        return model
    }
}

private struct PreviewTokens: AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct PreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}
#endif
