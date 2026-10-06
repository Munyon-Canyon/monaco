import Foundation

public enum ChatArrivals {
    public static func added(from previous: [ChatRow], to current: [ChatRow]) -> [ChatRow] {
        guard let newest = previous.last?.date else { return [] }
        let known = Set(previous.map(\.id))
        return current.filter { !known.contains($0.id) && $0.date >= newest }
    }

    public static func shouldFollow(added: [ChatRow], isFollowingThread: Bool) -> Bool {
        guard !added.isEmpty else { return false }
        return added.contains(where: \.isMine) || isFollowingThread
    }
}
