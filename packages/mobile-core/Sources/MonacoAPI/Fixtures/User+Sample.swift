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

extension Components.Schemas.CabalRef {
    public static let sampleBigBallers = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a805e", name: "Big Ballers", pictureUrl: nil)
}

extension Components.Schemas.UserCabals {
    public static let sample = Self(cabals: [
        Components.Schemas.UserCabal(
            cabal: .sampleACCapital, memberCount: 8, role: .admin, canVote: true, shared: true, ownershipBps: 2000),
        Components.Schemas.UserCabal(
            cabal: .sampleDegens, memberCount: 6, role: .member, canVote: true, shared: true, ownershipBps: 1500),
        Components.Schemas.UserCabal(
            cabal: .sampleBigBallers, memberCount: 12, role: .member, canVote: true, shared: false,
            ownershipBps: 800),
    ])

    public static let sampleHiddenPerformance = Self(
        cabals: sample.cabals.map { row in
            Components.Schemas.UserCabal(
                cabal: row.cabal, memberCount: row.memberCount, role: row.role, canVote: row.canVote,
                shared: row.shared, ownershipBps: nil)
        })
}

extension Components.Schemas.UserActivityCabal {
    public static let sampleAC = Self(id: "01890a5d-ac96-774b-bcce-b302099a8101", name: "AC Capital", pictureUrl: nil)
    public static let sampleDegens = Self(id: "01890a5d-ac96-774b-bcce-b302099a8102", name: "Degens", pictureUrl: nil)
    public static let sampleBigBallers = Self(
        id: "01890a5d-ac96-774b-bcce-b302099a8103", name: "Big Ballers", pictureUrl: nil)
}

extension Components.Schemas.UserActivity {
    private static let anchor: TimeInterval = 1_761_000_000

    public static let sampleVotedNVDA = Self.voted(
        Components.Schemas.UserActivityVoted(
            kind: .voted, at: Date(timeIntervalSince1970: anchor - 20 * 60), title: "Voted",
            cabal: .sampleAC, proposalId: "01890a5d-ac96-774b-bcce-b302099a8201", subject: "Buy NVDA",
            symbol: "NVDA", usdcMicros: 1_500_000_000, choice: nil))

    public static let sampleProposedMETA = Self.proposed(
        Components.Schemas.UserActivityProposed(
            kind: .proposed, at: Date(timeIntervalSince1970: anchor - 5 * 3_600), title: "Proposed",
            cabal: .sampleDegens, proposalId: "01890a5d-ac96-774b-bcce-b302099a8202", subject: "Buy META",
            symbol: "META", usdcMicros: 800_000_000, status: "passed", result: .init(yes: 4, no: 1)))

    public static let sampleReplied = Self.replied(
        Components.Schemas.UserActivityReplied(
            kind: .replied, at: Date(timeIntervalSince1970: anchor - 3 * 86_400), cabal: .sampleAC,
            commentId: "01890a5d-ac96-774b-bcce-b302099a8301", proposalId: "01890a5d-ac96-774b-bcce-b302099a8201",
            body: "Concentration is a feature", subject: "Buy NVDA", usdcMicros: 1_500_000_000))

    public static let sampleVotedSpaceX = Self.voted(
        Components.Schemas.UserActivityVoted(
            kind: .voted, at: Date(timeIntervalSince1970: anchor - 4 * 86_400), title: "Voted",
            cabal: .sampleBigBallers, proposalId: "01890a5d-ac96-774b-bcce-b302099a8203", subject: "Buy SpaceX",
            symbol: "SPCX", usdcMicros: 2_000_000_000, choice: nil))

    public static let sampleToppedBoard = Self.toppedBoard(
        Components.Schemas.UserActivityToppedBoard(
            kind: .toppedBoard, at: Date(timeIntervalSince1970: anchor - 21 * 86_400), title: "Topped the board",
            periodStart: "2026-09-01", right: 11, resolved: 12))

    public static let sampleOwnVoted = Self.voted(
        Components.Schemas.UserActivityVoted(
            kind: .voted, at: Date(timeIntervalSince1970: anchor - 20 * 60), title: "Voted",
            cabal: .sampleAC, proposalId: "01890a5d-ac96-774b-bcce-b302099a8201", subject: "Buy NVDA",
            symbol: "NVDA", usdcMicros: 1_500_000_000, choice: .yes))
}

extension Components.Schemas.UserActivityPage {
    public static let sampleChristelle = Self(
        items: [
            .sampleVotedNVDA, .sampleProposedMETA, .sampleReplied, .sampleVotedSpaceX, .sampleToppedBoard,
        ],
        nextCursor: nil
    )

    public static let sampleOwn = Self(
        items: [.sampleOwnVoted, .sampleProposedMETA, .sampleReplied, .sampleVotedSpaceX, .sampleToppedBoard],
        nextCursor: nil
    )

    public static let sampleEmpty = Self(items: [], nextCursor: nil)
}
#endif
