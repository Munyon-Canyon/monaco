import Foundation

public enum ActivityFeedCopy {
    public static let title = "Activity"
    public static let markAllRead = "Mark all read"
    public static let newLabel = "New"
    public static let earlierLabel = "Earlier"
    public static let emptyTitle = "You’re caught up"
    public static let emptyMessage = "Replies, votes and proposal updates appear here."
    public static let comingSoon = "Your activity shows up here soon."
    public static let loadingLabel = "Loading activity"
}

#if DEBUG
extension ActivityFeedCopy {
    enum Fixture {
        static let replyTitle = "Replied to your Apple proposal"
        static let replyBody = "How much cash would we have left after this?"
        static let replyCabal = "Big Ballers"
        static let replyAge = "2h"
        static let passedTitle = "The Microsoft proposal passed"
        static let passedBody = "The proposal reached its vote threshold."
        static let passedCabal = "Big Ballers"
        static let passedAge = "3h"
        static let passedBadge = "Execution pending"
        static let voteTitle = "Voted on your NVIDIA proposal"
        static let voteBody = "Voted Yes on your proposal."
        static let voteCabal = "Tech Crew"
        static let voteAge = "4h"
        static let depositTitle = "Deposit confirmed"
        static let depositAmount = "$100.00"
        static let depositAge = "Yesterday"
    }
}
#endif
