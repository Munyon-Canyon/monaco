import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalChatUnreadModel {
    public private(set) var unreadCount = 0
    public let cabalID: String

    private let api: APIClient
    private let hints: any HintSource
    private var generation = 0
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(cabalID: String, api: APIClient, hints: any HintSource) {
        self.cabalID = cabalID
        self.api = api
        self.hints = hints
    }

    public var hasUnread: Bool { unreadCount > 0 }

    public func load() async {
        generation += 1
        let issued = generation
        let cabalID = cabalID
        let cabals = try? await api.read { try await $0.getMyCabals().ok.body.json }
        guard issued == generation, !Task.isCancelled else { return }
        unreadCount = cabals?.first { $0.id == cabalID }?.unreadCount ?? 0
    }

    public func observe() async {
        await refresher.observe([
            hints.hints(matching: .cabal(id: cabalID, what: nil)),
            hints.hints(matching: .user(what: "cabals")),
        ])
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    public func markRead() {
        generation += 1
        unreadCount = 0
    }
}
