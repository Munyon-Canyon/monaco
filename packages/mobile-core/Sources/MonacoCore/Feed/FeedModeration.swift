import MonacoAPI
import MonacoSocial

public struct FeedReportTarget: Equatable, Sendable {
    public let kind: ReportKind
    public let targetID: String

    public init(kind: ReportKind, targetID: String) {
        self.kind = kind
        self.targetID = targetID
    }
}

public struct FeedBlockTarget: Equatable, Sendable {
    public let userID: String
    public let name: String

    public init(userID: String, name: String) {
        self.userID = userID
        self.name = name
    }
}

public enum FeedModeration {
    public static func reportTarget(for item: Components.Schemas.FeedItem, viewerID: String?) -> FeedReportTarget? {
        guard let actorID = otherActor(of: item, viewerID: viewerID) else { return nil }
        if item.refType == commentRefType {
            return FeedReportTarget(kind: .comment, targetID: item.refId)
        }
        return FeedReportTarget(kind: .user, targetID: actorID)
    }

    public static func blockTarget(for item: Components.Schemas.FeedItem, viewerID: String?) -> FeedBlockTarget? {
        otherActor(of: item, viewerID: viewerID).map { FeedBlockTarget(userID: $0, name: item.actorName ?? "") }
    }

    private static let commentRefType = "feed_comments"

    private static func otherActor(of item: Components.Schemas.FeedItem, viewerID: String?) -> String? {
        guard let actorID = item.actorId, actorID.caseInsensitiveCompare(viewerID ?? "") != .orderedSame else {
            return nil
        }
        return actorID
    }
}
