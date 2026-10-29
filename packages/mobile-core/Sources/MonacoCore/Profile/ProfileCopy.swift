import Foundation

public struct ProfileLine: Equatable, Sendable {
    public struct Run: Equatable, Sendable {
        public let text: String
        public let isStrong: Bool

        public init(_ text: String, isStrong: Bool = false) {
            self.text = text
            self.isStrong = isStrong
        }
    }

    public let runs: [Run]

    public init(_ runs: [Run]) {
        self.runs = runs
    }

    public var plain: String { runs.map(\.text).joined() }
}

public enum ProfileCopy {
    public static let positionsLabel = "Positions"
    public static let pnlLabel = "P&L · 1M"
    public static let positionsInfoTitle = "Positions"
    public static let positionsInfo = "Only you see your positions and cash. Others see your P&L as a percent."
    public static let cashLabel = MoneyCopy.cashLabel
    public static let apyLabel = CabalYield.apy
    public static let apyInfoTitle = CabalYield.apy
    public static let apyInfo = CabalYield.fundNote
    public static let depositLabel = "Deposit"
    public static let editPhotoLabel = "Edit profile photo"
    public static let editDetailsLabel = "Edit name and profile details"
    public static let failedTitle = "Couldn\u{2019}t load this view"
    public static let tryAgain = "Try again"

    private static let separator = " \u{00B7} "

    public static func identityLine(
        handle: String?, joinedAt: Date, timeZone: TimeZone = .current, locale: Locale = .current
    )
        -> String
    {
        let formatter = DateFormatter()
        formatter.locale = locale
        formatter.timeZone = timeZone
        formatter.setLocalizedDateFormatFromTemplate("MMM yyyy")
        let joined = "joined \(formatter.string(from: joinedAt))"
        guard let handle, !handle.isEmpty else { return joined }
        return "@\(handle)\(separator)\(joined)"
    }

    public static func counts(followers: Int, following: Int, cabals: Int) -> ProfileLine {
        ProfileLine([
            .init(FollowCountFormatter.count(followers), isStrong: true),
            .init(" \(FollowCountFormatter.followersNoun(followers))\(separator)"),
            .init(FollowCountFormatter.count(following), isStrong: true),
            .init(" following\(separator)"),
            .init(FollowCountFormatter.count(cabals), isStrong: true),
            .init(cabals == 1 ? " cabal" : " cabals"),
        ])
    }

    public static func standing(_ standing: OwnStanding) -> ProfileLine {
        switch standing {
        case .ranked(let ratePercent, let right, let resolved, let rank, let rankedCount):
            ProfileLine([
                .init("\(ratePercent)%", isStrong: true),
                .init(" right calls this month\(separator)"),
                .init("\(right) of \(resolved)", isStrong: true),
                .init(separator),
                .init("#\(rank)", isStrong: true),
                .init(" of \(rankedCount)"),
            ])
        case .unranked(let resolved, let moreToRank):
            ProfileLine([
                .init("\(resolved)", isStrong: true),
                .init(" resolved this month\(separator)"),
                .init("\(moreToRank)", isStrong: true),
                .init(" more to rank"),
            ])
        }
    }
}
