#if DEBUG
import Foundation

extension Components.Schemas.PublicProfile {
    public static let sample = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "maya", displayName: "Maya Angelou",
        photoUrl: "https://cdn.example.com/photos/maya.jpg", bio: "Poet.\nSan Francisco",
        createdAt: Date(timeIntervalSince1970: 1_759_233_600), cabalCount: 3, followerCount: 12, followingCount: 8,
        followedByMe: false, blockedByMe: false, return1mBps: 820, pnl1mMicros: 41_250_000, returnPeriod: ._1m,
        returnPrivate: false, standing: .sampleRanked, crown: false, cabalsInCommon: 2
    )

    public static let sampleOwn = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8070", handle: "andres", displayName: "Andres",
        photoUrl: nil, bio: nil, createdAt: Date(timeIntervalSince1970: 1_759_233_600), cabalCount: 3,
        followerCount: 12, followingCount: 8, followedByMe: false, blockedByMe: false, return1mBps: 410,
        pnl1mMicros: 241_100_000, returnPeriod: ._1m, returnPrivate: true, standing: .sampleRanked, crown: false,
        cabalsInCommon: 0
    )

    public static let sampleLeader = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8071", handle: "christelle", displayName: "Christelle",
        photoUrl: nil, bio: nil, createdAt: Date(timeIntervalSince1970: 1_759_233_600), cabalCount: 3,
        followerCount: 40, followingCount: 15, followedByMe: false, blockedByMe: false, return1mBps: 1_240,
        pnl1mMicros: 520_000_000, returnPeriod: ._1m, returnPrivate: false, standing: .sampleLeader, crown: true,
        cabalsInCommon: 2
    )

    public static let sampleUnranked = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8072", handle: "noor", displayName: "Noor",
        photoUrl: nil, bio: nil, createdAt: Date(timeIntervalSince1970: 1_759_233_600), cabalCount: 1,
        followerCount: 2, followingCount: 3, followedByMe: false, blockedByMe: false, return1mBps: nil,
        pnl1mMicros: nil, returnPeriod: ._1m, returnPrivate: false, standing: .sampleUnranked, crown: false,
        cabalsInCommon: 1
    )
}

extension Components.Schemas.PublicProfile.StandingPayload {
    public static let sampleRanked = Self(
        period: .month, periodStart: "2026-10-01", rateBps: 7_800, right: 7, resolved: 9, rank: 8, rankedCount: 24,
        moreToRank: 0
    )

    public static let sampleLeader = Self(
        period: .month, periodStart: "2026-10-01", rateBps: 9_200, right: 12, resolved: 13, rank: 1, rankedCount: 24,
        moreToRank: 0
    )

    public static let sampleUnranked = Self(
        period: .month, periodStart: "2026-10-01", rateBps: nil, right: 2, resolved: 3, rank: nil, rankedCount: 24,
        moreToRank: 2
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
