import Foundation

public struct ChatScrollTracker: Equatable, Sendable {
    public struct Position: Equatable, Sendable {
        public var offset: Double
        public var isAtEnd: Bool

        public init(offset: Double, isAtEnd: Bool) {
            self.offset = offset
            self.isAtEnd = isAtEnd
        }
    }

    private struct DragOrigin: Equatable, Sendable {
        let offset: Double
        let wasFollowing: Bool
    }

    public static let pinnedSlack = 40.0

    public private(set) var readerControlsScroll = false
    public private(set) var unreadCount = 0
    private var position = Position(offset: 0, isAtEnd: true)
    private var dragOrigin: DragOrigin?

    public init() {}

    public var isFollowingThread: Bool { !readerControlsScroll }

    public mutating func positionChanged(_ updated: Position) {
        position = updated
        if updated.isAtEnd { returnToEnd() }
    }

    public mutating func dragBegan() {
        if dragOrigin == nil {
            dragOrigin = DragOrigin(offset: position.offset, wasFollowing: isFollowingThread)
        }
        readerControlsScroll = true
    }

    public mutating func scrollSettled() {
        let origin = dragOrigin
        dragOrigin = nil
        if position.isAtEnd {
            returnToEnd()
            return
        }
        guard let origin, origin.wasFollowing, abs(position.offset - origin.offset) <= Self.pinnedSlack else { return }
        returnToEnd()
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
        returnToEnd()
        return true
    }

    private mutating func returnToEnd() {
        readerControlsScroll = false
        unreadCount = 0
    }
}
