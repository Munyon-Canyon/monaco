import Foundation

public struct ChatScrollTracker: Equatable, Sendable {
    public struct Position: Equatable, Sendable {
        public var distanceFromEnd: Double

        public var isAtEnd: Bool { distanceFromEnd <= ChatScrollTracker.pinnedSlack }

        public init(distanceFromEnd: Double) {
            self.distanceFromEnd = distanceFromEnd
        }
    }

    private struct DragOrigin: Equatable, Sendable {
        let distanceFromEnd: Double
        let wasFollowing: Bool
        var fingerLiftedInsideSlack = false
    }

    public static let pinnedSlack = 40.0

    public private(set) var readerControlsScroll = false
    public private(set) var unreadCount = 0
    private var position = Position(distanceFromEnd: 0)
    private var dragOrigin: DragOrigin?

    public init() {}

    public var isFollowingThread: Bool { !readerControlsScroll }

    public mutating func positionChanged(_ updated: Position) {
        position = updated
        if updated.isAtEnd {
            returnToEnd()
            return
        }
        guard let origin = dragOrigin, origin.wasFollowing, !origin.fingerLiftedInsideSlack else { return }
        if updated.distanceFromEnd - origin.distanceFromEnd > Self.pinnedSlack { readerControlsScroll = true }
    }

    public mutating func dragBegan() {
        if dragOrigin == nil || dragOrigin?.fingerLiftedInsideSlack == true {
            dragOrigin = DragOrigin(distanceFromEnd: position.distanceFromEnd, wasFollowing: isFollowingThread)
        }
        if dragOrigin?.wasFollowing != true { readerControlsScroll = true }
    }

    public mutating func dragEnded() {
        guard var origin = dragOrigin, origin.wasFollowing, isFollowingThread else { return }
        origin.fingerLiftedInsideSlack = true
        dragOrigin = origin
    }

    @discardableResult
    public mutating func scrollSettled() -> Bool {
        let origin = dragOrigin
        dragOrigin = nil
        if position.isAtEnd {
            returnToEnd()
            return false
        }
        guard let origin, origin.wasFollowing else { return false }
        let moved = position.distanceFromEnd - origin.distanceFromEnd
        guard origin.fingerLiftedInsideSlack || moved <= Self.pinnedSlack else { return false }
        returnToEnd()
        return true
    }

    public mutating func historyRequested() {
        readerControlsScroll = true
    }

    public mutating func followRequested() {
        returnToEnd()
    }

    public mutating func arrived(_ added: [ChatRow]) -> Bool {
        guard !added.isEmpty else { return false }
        guard ChatArrivals.shouldFollow(added: added, isFollowingThread: isFollowingThread) else {
            unreadCount += added.count
            return false
        }
        let wasFollowing = isFollowingThread
        returnToEnd()
        return !(wasFollowing && added.contains(where: \.isMine))
    }

    private mutating func returnToEnd() {
        readerControlsScroll = false
        unreadCount = 0
    }
}
