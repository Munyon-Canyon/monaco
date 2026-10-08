#if DEBUG
import Foundation

extension Components.Schemas.PublicProfile {
    public static let sample = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "maya", displayName: "Maya Angelou",
        photoUrl: "https://cdn.example.com/photos/maya.jpg", followerCount: 12, followingCount: 8, followedByMe: false,
        blockedByMe: false
    )
}

extension Components.Schemas.FollowUser {
    public static let sample = Self(
        userId: "01890a5d-ac96-774b-bcce-b302099a8059", handle: "kai", displayName: "Kai Cenat",
        photoUrl: nil, followedByMe: false
    )
}

extension Components.Schemas.FollowsPage {
    public static let sampleCursor = "sample-cursor-2"

    public static let sampleFirst = Self(
        items: [
            .sample,
            Components.Schemas.FollowUser(
                userId: "01890a5d-ac96-774b-bcce-b302099a805a", handle: "bea", displayName: "Bea", photoUrl: nil,
                followedByMe: true),
        ],
        nextCursor: sampleCursor
    )

    public static let sampleLast = Self(
        items: [
            Components.Schemas.FollowUser(
                userId: "01890a5d-ac96-774b-bcce-b302099a805b", handle: "cam", displayName: "Cam", photoUrl: nil,
                followedByMe: false)
        ],
        nextCursor: nil
    )

    public static let sampleEmpty = Self(items: [], nextCursor: nil)
}
#endif
