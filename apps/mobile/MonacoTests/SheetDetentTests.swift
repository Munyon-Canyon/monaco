import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct SheetDetentTests {
    private let kinds: [MonacoSheetDetents] = [.review, .votes, .result, .fitted]

    @Test func noSheetKindOffersTheLargeDetent() {
        for kind in kinds {
            for fitted in [0, 180, 900] as [CGFloat] {
                #expect(!kind.detents(fittedHeight: fitted).contains(.large))
                #expect(kind.detents(fittedHeight: fitted).count == 1)
            }
        }
    }

    @Test func reviewAndVotesOpenAtHalfTheScreen() {
        #expect(MonacoSheetDetents.review.detents(fittedHeight: 0) == [.fraction(0.55)])
        #expect(MonacoSheetDetents.votes.detents(fittedHeight: 0) == [.fraction(0.55)])
    }

    @Test func depositConfirmedKeepsItsFixedHeight() {
        #expect(MonacoSheetDetents.result.detents(fittedHeight: 0) == [.height(310)])
    }

    @Test func aFittedSheetUsesItsContentHeightOrHalfWhileMeasuring() {
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 240) == [.height(240)])
        #expect(MonacoSheetDetents.fitted.detents(fittedHeight: 0) == [.fraction(0.55)])
    }
}

@MainActor
struct DepositSheetTests {
    @Test func presentingDepositDoesNotPushAScreen() {
        let navigator = AppNavigator()
        navigator.presentDeposit(prefillMicros: 5_000_000, cabalID: "c1")
        #expect(navigator.deposit?.prefillMicros == 5_000_000)
        #expect(navigator.deposit?.cabalID == "c1")
        #expect(navigator.homePath.isEmpty)
    }

    @Test func choosingAnOptionClosesTheSheetAndPushesOnTheCurrentTab() {
        let navigator = AppNavigator()
        navigator.selectedTab = .profile
        navigator.presentDeposit()
        let route = AnyAppRoute(CardFundingRoute(prefillMicros: nil, cabalID: nil))
        navigator.continueDeposit(with: route)
        #expect(navigator.deposit == nil)
        #expect(navigator.profilePath == [route])
        #expect(navigator.homePath.isEmpty)
    }
}
