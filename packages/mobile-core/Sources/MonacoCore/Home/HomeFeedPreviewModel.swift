import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class HomeFeedPreviewModel {
    public enum Source: Equatable, Sendable {
        case yourCabals
        case everyone

        var scope: FeedScope {
            switch self {
            case .yourCabals: .mine
            case .everyone: .others
            }
        }

        var limit: Int {
            switch self {
            case .yourCabals: 5
            case .everyone: 4
            }
        }
    }

    public let source: Source
    public private(set) var state: LoadState<[Components.Schemas.FeedItem]> = .idle

    private let api: APIClient
    private let hints: any HintSource
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource, clock: any Clock<Duration>, source: Source) {
        self.api = api
        self.hints = hints
        self.source = source
    }

    public func load() async {
        if case .idle = state { state = .loading }
        let query = FeedQuery(scope: source.scope).parameters(cursor: nil, limit: source.limit)
        do {
            let page = try await api.read { client in try await client.getFeed(query: query).ok.body.json }
            state = .loaded(page.items)
        } catch {
            if case .loaded = state { return }
            state = .failed(APIError(error))
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "feed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
