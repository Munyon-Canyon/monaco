import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

struct ShareCardButtonsTests {
    private static let summary = PortfolioSummary(Components.Schemas.MyPortfolio.sample)

    @Test func aMemberOfAnActiveCabalCanShare() {
        let cabal = Components.Schemas.Cabal.sample(role: "member")

        #expect(CabalScreen.canShare(cabal: cabal))
    }

    @Test func aNonMemberCannotShareACabal() {
        let cabal = Components.Schemas.Cabal.sample(role: nil)

        #expect(!CabalScreen.canShare(cabal: cabal))
    }

    @Test func aClosedCabalCannotBeShared() {
        var cabal = Components.Schemas.Cabal.sample(role: "member")
        cabal.status = "banned"

        #expect(!CabalScreen.canShare(cabal: cabal))
    }

    @Test func aCabalThatHasNotLoadedCannotBeShared() {
        #expect(!CabalScreen.canShare(cabal: nil))
    }

    @Test func yourReturnCanBeSharedOnceYouAreSignedInAndTheFiguresLoaded() {
        let profile = SessionProfile(Components.Schemas.Me.sample)

        #expect(ProfileScreen.canShare(profile: profile, portfolioState: .loaded(Self.summary)))
        #expect(!ProfileScreen.canShare(profile: nil, portfolioState: .loaded(Self.summary)))
        #expect(!ProfileScreen.canShare(profile: profile, portfolioState: .loading))
        #expect(!ProfileScreen.canShare(profile: profile, portfolioState: nil))
    }

    @Test @MainActor func theCabalShareOpensTheStakeCardOnTheGivenTab() {
        let navigator = AppNavigator()

        ShareCardLauncher.shareCabal(id: "c1", navigator: navigator, in: .cabals)

        #expect(navigator.cabalsPath.last == AnyAppRoute(ShareRoute(subject: .stake(cabalID: "c1"))))
    }

    @Test @MainActor func theProfileShareOpensThePortfolioCardOnTheProfileTab() {
        let navigator = AppNavigator()

        ShareCardLauncher.shareMe(navigator: navigator, in: .profile)

        #expect(navigator.profilePath.last == AnyAppRoute(ShareRoute(subject: .portfolio)))
    }

    private static var pot: CabalPotSummary {
        CabalPotSummary(Components.Schemas.CabalPot.sampleInvested)
    }
}
