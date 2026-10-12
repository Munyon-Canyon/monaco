#if DEBUG
import Foundation
import MonacoAPI
import MonacoCore

enum ShareCardSamples {
    enum Mood {
        case gain
        case loss
    }

    static var cabalGain: ShareCardContent { cabal(.gain) }

    static func cabal(_ mood: Mood) -> ShareCardContent {
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: "member")
        cabal.name = "Sunday Investors"
        cabal.memberCount = 12
        cabal.rules.joinMode = "open"
        let gain = mood == .gain
        let pnl: Int64 = gain ? 139_893_540_000 : -2_840_000_000
        let putIn: Int64 = 7_960_000_000 + (gain ? 0 : 2_000_000_000)
        let pot = Components.Schemas.CabalPot.sample(
            potValueMicros: putIn + pnl, cashMicros: putIn + pnl, cashWeightBps: 10000, pnlMicros: pnl,
            returnBps: gain ? 175_745 : -2_840, holdings: [], me: nil)
        var history = Components.Schemas.CabalValueHistory.sample(cabalID: cabal.id, range: .all)
        history.points = ShareCardSampleSeries.permille(gain: gain).map { offset, permille in
            Components.Schemas.CabalValuePoint(
                at: start.addingTimeInterval(offset), valueMicros: putIn + pnl * permille / 1000,
                navPerShareMicros: 1_000_000, pnlMicros: pnl * permille / 1000)
        }
        return unwrap(
            ShareCardContent.cabal(
                cabal, pot: pot, history: history, links: links, locale: locale, timeZone: timeZone))
    }

    static func me(_ mood: Mood, links: InviteLinks? = Self.links) -> ShareCardContent {
        let gain = mood == .gain
        let pnl: Int64 = gain ? 2_483_120_000 : -412_500_000
        let total: Int64 = 9_812_400_000
        var portfolio = gain ? Components.Schemas.MyPortfolio.sample : .sampleLoss
        portfolio.totalValueMicros = total
        portfolio.pnlMicros = pnl
        portfolio.returnBps = gain ? 3_387 : -403
        var history = Components.Schemas.MyPnlHistory.sample(range: .all)
        history.points = ShareCardSampleSeries.permille(gain: gain).map { offset, permille in
            Components.Schemas.PnlPoint(
                at: start.addingTimeInterval(offset), equityMicros: total - pnl + pnl * permille / 1000,
                pnlMicros: pnl * permille / 1000)
        }
        return ShareCardContent.me(
            profile: SessionProfile(Components.Schemas.Me.sample), portfolio: portfolio, history: history,
            links: links, locale: locale, timeZone: timeZone)
    }

    static var cabalShort: ShareCardContent {
        var cabal = Components.Schemas.Cabal.sampleWithMembers(role: "member")
        cabal.rules.joinMode = "open"
        return unwrap(
            ShareCardContent.cabal(
                cabal, pot: .sampleInvested, history: .sampleShort(), links: nil, locale: locale, timeZone: timeZone))
    }

    private static let start = Date(timeIntervalSince1970: 1_780_000_000)
    private static let locale = Locale(identifier: "en_US")
    private static let timeZone = TimeZone(identifier: "UTC") ?? .current
    private static let links = InviteLinks(Components.Schemas.MyReferralCode.sample)

    private static func unwrap(_ content: ShareCardContent?) -> ShareCardContent {
        guard let content else { preconditionFailure("sample share card is nil") }
        return content
    }
}
#endif
