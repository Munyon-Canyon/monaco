import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

struct ShareCardButtonsTests {
    private static let summary = PortfolioSummary(Components.Schemas.MyPortfolio.sample)

    @Test func aMemberOfAnActiveCabalWithALoadedPotCanShare() {
        let cabal = Components.Schemas.Cabal.sample(role: "member")

        #expect(CabalScreen.canShare(cabal: cabal, potState: .loaded(Self.pot)))
    }

    @Test func aNonMemberCannotShareACabal() {
        let cabal = Components.Schemas.Cabal.sample(role: nil)

        #expect(!CabalScreen.canShare(cabal: cabal, potState: .loaded(Self.pot)))
    }

    @Test func aClosedCabalCannotBeShared() {
        var cabal = Components.Schemas.Cabal.sample(role: "member")
        cabal.status = "banned"

        #expect(!CabalScreen.canShare(cabal: cabal, potState: .loaded(Self.pot)))
    }

    @Test func aCabalWhoseFiguresAreStillLoadingCannotBeShared() {
        let cabal = Components.Schemas.Cabal.sample(role: "creator")

        #expect(!CabalScreen.canShare(cabal: cabal, potState: .loading))
        #expect(!CabalScreen.canShare(cabal: cabal, potState: .idle))
        #expect(!CabalScreen.canShare(cabal: nil, potState: .loaded(Self.pot)))
    }

    @Test func yourReturnCanBeSharedOnceYouAreSignedInAndTheFiguresLoaded() {
        let profile = SessionProfile(Components.Schemas.Me.sample)

        #expect(ProfileScreen.canShare(profile: profile, portfolioState: .loaded(Self.summary)))
        #expect(!ProfileScreen.canShare(profile: nil, portfolioState: .loaded(Self.summary)))
        #expect(!ProfileScreen.canShare(profile: profile, portfolioState: .loading))
        #expect(!ProfileScreen.canShare(profile: profile, portfolioState: nil))
    }

    private static var pot: CabalPotSummary {
        CabalPotSummary(Components.Schemas.CabalPot.sampleInvested)
    }
}

struct ShareCardSheetCopyTests {
    @Test func theSheetSpeaksInTheWordsOfTheInviteScreen() {
        #expect(ShareCardCopy.title == "Share")
        #expect(ShareCardCopy.share == "Share")
        #expect(ShareCardCopy.copyLink == "Copy link")
        #expect(ShareCardCopy.copied == InviteCopy.copied)
        #expect(ShareCardCopy.copied == "Link copied.")
    }
}
