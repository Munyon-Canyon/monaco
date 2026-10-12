import Foundation
import MonacoAPI

public struct ShareCardContent: Equatable, Sendable {
    public enum Kind: Equatable, Sendable {
        case cabal
        case me
    }

    public struct Stat: Equatable, Sendable {
        public let label: String
        public let value: String

        public init(label: String, value: String) {
            self.label = label
            self.value = value
        }
    }

    public static let fallbackURL = URL(string: "https://monacolabs.xyz") ?? URL(fileURLWithPath: "/")
    public static let fallbackFooterLink = "monacolabs.xyz"

    public let kind: Kind
    public let title: String
    public let subtitle: String
    public let status: String
    public let dateText: String?
    public let points: [CurvePoint]
    public let direction: PortfolioSummary.Direction
    public let gain: String
    public let returnText: String
    public let stats: [Stat]
    public let footerLink: String
    public let shareURL: URL
    public let spoken: String
    public let faceSeed: String
    public let faceName: String
    public let faceURL: URL?

    public init(
        kind: Kind, title: String, subtitle: String, status: String, dateText: String?, points: [CurvePoint],
        direction: PortfolioSummary.Direction, gain: String, returnText: String, stats: [Stat], footerLink: String,
        shareURL: URL, spoken: String, faceSeed: String, faceName: String, faceURL: URL?
    ) {
        self.kind = kind
        self.title = title
        self.subtitle = subtitle
        self.status = status
        self.dateText = dateText
        self.points = points
        self.direction = direction
        self.gain = gain
        self.returnText = returnText
        self.stats = stats
        self.footerLink = footerLink
        self.shareURL = shareURL
        self.spoken = spoken
        self.faceSeed = faceSeed
        self.faceName = faceName
        self.faceURL = faceURL
    }

    public static func cabal(
        _ cabal: Components.Schemas.Cabal,
        pot: Components.Schemas.CabalPot,
        history: Components.Schemas.CabalValueHistory,
        links: InviteLinks?,
        locale: Locale = .current,
        timeZone: TimeZone = .current
    ) -> ShareCardContent? {
        guard cabal.status != "banned" else { return nil }
        let members = Int(cabal.memberCount)
        let status = cabal.rules.joinMode == "open" ? "Open" : "By request"
        let putIn = pot.potValueMicros - pot.pnlMicros
        let inThePot = UsdAmountFormatter.format(micros: pot.potValueMicros)
        let subtitle = Self.members(members)
        let figures = Figures(pnlMicros: pot.pnlMicros, returnBps: pot.returnBps.map(Int64.init))
        return ShareCardContent(
            kind: .cabal, title: cabal.name, subtitle: subtitle, status: status,
            dateText: Self.date(pot.pricesAsOf, locale: locale, timeZone: timeZone),
            points: history.points.map { CurvePoint(at: $0.at, value: $0.pnlMicros) },
            direction: figures.direction, gain: figures.gain, returnText: figures.returnText,
            stats: [
                Stat(label: "Put in", value: UsdAmountFormatter.format(micros: putIn)),
                Stat(label: "In the pot", value: inThePot),
                Stat(label: "Members", value: "\(members)"),
            ],
            footerLink: Self.footerLink(links), shareURL: links?.shareURL ?? fallbackURL,
            spoken: Self.spoken(
                lead: "\(cabal.name), \(status == "Open" ? "open" : "by request") cabal, \(subtitle)", figures: figures,
                tail: "Put in \(UsdAmountFormatter.format(micros: putIn)), in the pot \(inThePot)."),
            faceSeed: cabal.id, faceName: cabal.name, faceURL: cabal.pictureUrl.flatMap(URL.init(string:)))
    }

    public static func me(
        profile: SessionProfile,
        portfolio: Components.Schemas.MyPortfolio,
        history: Components.Schemas.MyPnlHistory,
        links: InviteLinks?,
        locale: Locale = .current,
        timeZone: TimeZone = .current
    ) -> ShareCardContent {
        let putIn = portfolio.totalValueMicros - portfolio.pnlMicros
        let inCabals = UsdAmountFormatter.format(micros: portfolio.totalValueMicros)
        let count = portfolio.cabals.count
        let figures = Figures(pnlMicros: portfolio.pnlMicros, returnBps: portfolio.returnBps)
        let subtitle = profile.handle.map { "@\($0)" } ?? ""
        return ShareCardContent(
            kind: .me, title: profile.displayName, subtitle: subtitle,
            status: MemberSinceFormatter.format(profile.createdAt, timeZone: timeZone, locale: locale),
            dateText: portfolio.pricesAsOf.flatMap { Self.date($0, locale: locale, timeZone: timeZone) },
            points: history.points.map { CurvePoint(at: $0.at, value: $0.pnlMicros) },
            direction: figures.direction, gain: figures.gain, returnText: figures.returnText,
            stats: [
                Stat(label: "Put in", value: UsdAmountFormatter.format(micros: putIn)),
                Stat(label: "In cabals", value: inCabals),
                Stat(label: "Cabals", value: "\(count)"),
            ],
            footerLink: Self.footerLink(links), shareURL: links?.shareURL ?? fallbackURL,
            spoken: Self.spoken(
                lead: [profile.displayName, subtitle].filter { !$0.isEmpty }.joined(separator: ", "),
                figures: figures,
                tail: "Put in \(UsdAmountFormatter.format(micros: putIn)), in cabals \(inCabals)."),
            faceSeed: profile.userID, faceName: profile.displayName, faceURL: profile.photoURL)
    }

    private struct Figures {
        let direction: PortfolioSummary.Direction
        let gain: String
        let returnText: String

        init(pnlMicros: Int64, returnBps: Int64?) {
            direction = PortfolioSummary.Direction(signedMicros: pnlMicros)
            gain = UsdAmountFormatter.format(signedMicros: pnlMicros)
            returnText = PortfolioSummary.percent(returnBps)
        }

        var spoken: String {
            let word =
                switch direction {
                case .up: "Up"
                case .down: "Down"
                case .flat: "No change"
                }
            let amount = gain.trimmingCharacters(in: CharacterSet(charactersIn: "+\u{2212}"))
            let percent = returnText.trimmingCharacters(in: CharacterSet(charactersIn: "+\u{2212}"))
            let tail = returnText == "—" ? "" : ", \(percent.dropLast()) percent"
            return direction == .flat ? "\(word), all time." : "\(word) \(amount)\(tail), all time."
        }
    }

    private static func spoken(lead: String, figures: Figures, tail: String) -> String {
        "\(lead). \(figures.spoken) \(tail)"
    }

    private static func members(_ count: Int) -> String {
        count == 1 ? "1 member" : "\(count) members"
    }

    private static func footerLink(_ links: InviteLinks?) -> String {
        links.map { InviteLinks.withoutScheme($0.shareURL) } ?? fallbackFooterLink
    }

    private static func date(_ date: Date, locale: Locale, timeZone: TimeZone) -> String {
        let formatter = DateFormatter()
        formatter.locale = locale
        formatter.timeZone = timeZone
        formatter.setLocalizedDateFormatFromTemplate("MMM d, yyyy")
        return formatter.string(from: date)
    }
}
