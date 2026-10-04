import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct HomeBalanceDisplayTests {
    private func balance(micros: Int64, inFlight: Int64 = 0) -> AccountBalance {
        AccountBalance(
            availableMicros: micros, onChainMicros: micros + inFlight, inFlightMicros: inFlight,
            depositAddress: "wallet-1", asOf: Date(timeIntervalSince1970: 1_759_579_200)
        )
    }

    @Test func aLoadedBalanceShowsItsAmount() {
        #expect(HomeBalanceDisplay.resolve(.loaded(balance(micros: 248_500_000))) == .amount(248_500_000))
    }

    @Test func aFirstLoadInFlightShowsLoading() {
        #expect(HomeBalanceDisplay.resolve(.idle) == .loading)
        #expect(HomeBalanceDisplay.resolve(.loading) == .loading)
    }

    @Test func aFailedBalanceIsUnavailableNotZero() {
        #expect(HomeBalanceDisplay.resolve(.failed(.transport(URLError(.notConnectedToInternet)))) == .unavailable)
    }

    @Test func aRealZeroBalanceStillShowsZero() {
        #expect(HomeBalanceDisplay.resolve(.loaded(balance(micros: 0))) == .amount(0))
    }

    @Test func onlyAFirstLoadFailureShowsTheRetryRow() {
        #expect(HomeBalanceRowSection.showsRetryRow(.failed(.transport(URLError(.notConnectedToInternet)))))
        #expect(!HomeBalanceRowSection.showsRetryRow(.loaded(balance(micros: 248_500_000))))
        #expect(!HomeBalanceRowSection.showsRetryRow(.loading))
        #expect(!HomeBalanceRowSection.showsRetryRow(.idle))
    }

    @Test func theRowSaysWhatIsFundingACabal() {
        let row = PlatformBalanceCard(state: .loaded(balance(micros: 198_500_000, inFlight: 50_000_000)))
        #expect(row.pendingAllocationMicros == 50_000_000)
        #expect(PlatformBalanceCard(state: .loading).pendingAllocationMicros == 0)
    }
}
