public enum LeaderboardCopy {
    public static let title = "Leaderboard"
    public static let categoryTitle = "Leaderboard category"
    public static let periodTitle = "Leaderboard period"
    public static let filterLabel = "Filter and sort leaderboard"
    public static let filtersOn = "Filters on"
    public static let infoLabel = "About this ranking"

    public static let filterSheetTitle = "Leaderboard filters"
    public static let sortTitle = "Sort by"
    public static let membershipTitle = "Membership"
    public static let minimumMembersTitle = "Minimum members"
    public static let scopeTitle = "Show"
    public static let apply = "Apply filters"
    public static let reset = "Reset"

    public static let aboutSecondary =
        "Dollar P&L and percentage return share the selected period. Deposits are not gains. Only permitted public performance is ranked."

    public static let stale = "Rankings paused while valuations update."
    public static let emptyTitle = "No eligible rankings"
    public static let emptyMessage = "No public data for this period."
    public static let followingEmptyTitle = "Follow people to see how they do."
    public static let findFriends = "Find friends"
    public static let filteredEmptyTitle = "No matching rankings"
    public static let filteredEmptyMessage = "Try removing a filter."
    public static let resetFilters = "Reset filters"
    public static let errorTitle = "Couldn’t load this view"
    public static let errorMessage =
        "Your existing information hasn’t been changed. Try again when your connection is available."
    public static let tryAgain = "Try again"
    public static let loadingMore = "Couldn't load more rankings."
    public static let movementNotCompared = "Rank movement not compared for this sort"

    public static func contextLine(board: LeaderboardBoardKind, range: LeaderboardRange, hasMovement: Bool) -> String {
        let subject = board == .cabals ? "Cabal P&L" : "Shared personal P&L"
        let tail = hasMovement ? "Rank change vs previous snapshot" : "Rank change opens soon"
        return "\(subject) · \(range.label) · \(tail)"
    }

    public static func allShown(board: LeaderboardBoardKind, count: Int) -> String {
        "All \(count) eligible \(board == .cabals ? "cabals" : "people") shown"
    }

    public static func members(_ count: Int) -> String {
        count == 1 ? "1 member" : "\(count) members"
    }

    public static func spoken(_ movement: LeaderboardMovement) -> String {
        switch movement {
        case .up(let places): "Up \(places) \(places == 1 ? "place" : "places") from the previous snapshot"
        case .down(let places): "Down \(places) \(places == 1 ? "place" : "places") from the previous snapshot"
        case .unchanged: "Unchanged"
        case .new: "New on the board"
        }
    }
}
