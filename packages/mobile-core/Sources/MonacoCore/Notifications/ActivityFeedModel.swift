import Foundation
import Observation

public enum ActivityFeedOutcome: Equatable, Sendable {
    case items([ActivityFeedItem])
    case failed
    case comingSoon
}

public protocol ActivityFeedSource: Sendable {
    func load() async -> ActivityFeedOutcome
}

public struct LiveActivityFeedSource: ActivityFeedSource {
    public init() {}

    public func load() async -> ActivityFeedOutcome { .comingSoon }
}

@Observable
@MainActor
public final class ActivityFeedModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case failed
        case empty
        case loaded
        case comingSoon
    }

    public private(set) var phase: Phase = .loading
    public private(set) var items: [ActivityFeedItem] = []

    private let source: any ActivityFeedSource

    public init(source: any ActivityFeedSource = LiveActivityFeedSource()) {
        self.source = source
    }

    public var hasUnread: Bool { items.contains { !$0.isRead } }

    public var groupLabel: String { hasUnread ? ActivityFeedCopy.newLabel : ActivityFeedCopy.earlierLabel }

    public func load() async {
        if items.isEmpty { phase = .loading }
        switch await source.load() {
        case .items(let loaded):
            items = loaded
            phase = loaded.isEmpty ? .empty : .loaded
        case .failed:
            if items.isEmpty { phase = .failed }
        case .comingSoon:
            items = []
            phase = .comingSoon
        }
    }

    public func markAllRead() {
        for index in items.indices { items[index].isRead = true }
    }

    public func markRead(id: ActivityFeedItem.ID) {
        guard let index = items.firstIndex(where: { $0.id == id }) else { return }
        items[index].isRead = true
    }
}
