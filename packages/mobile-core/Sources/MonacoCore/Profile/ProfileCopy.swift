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
    public static let inCabalsLabel = "In cabals"
    public static let monthLabel = "This month"
    public static let rightMonthLabel = "Right this month"
    public static let pnlLabel = monthLabel
    public static let privacyHint = "Only you see these numbers. Others see your P&L as a percent."
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

    public static func ordinal(_ n: Int) -> String {
        let suffix =
            if (11...13).contains(n % 100) {
                "th"
            } else {
                switch n % 10 {
                case 1: "st"
                case 2: "nd"
                case 3: "rd"
                default: "th"
                }
            }
        return "\(n)\(suffix)"
    }

    public static func standing(_ standing: OwnStanding) -> ProfileLine {
        switch standing {
        case .ranked(_, let right, let resolved, let rank, let rankedCount):
            ProfileLine([
                .init("\(right) of \(resolved)", isStrong: true),
                .init(" right this month\(separator)"),
                .init(ordinal(rank), isStrong: true),
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
