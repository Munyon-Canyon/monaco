import MonacoAPI
import SwiftUI
import Testing

@testable import Monaco
@testable import MonacoCore

struct BoardRowTests {
    private func row(
        rank: Int32, returnBps: Int64? = 450, pnlMicros: Int64 = 2_000_000,
        flags: [Components.Schemas.LeaderboardRow.FlagsPayloadPayload] = []
    ) -> LeaderboardRowView {
        LeaderboardRowView(
            .sample(
                rank: rank, id: "u\(rank)", name: "Ana", valueMicros: 50_000_000, pnlMicros: pnlMicros,
                returnBps: returnBps, flags: flags))
    }

    @Test func firstPlaceIsSpokenAsFirst() {
        #expect(
            BoardRow<EmptyView>.spoken(row(rank: 1), figure: .pnl) == "First, Ana, up 4.50 percent, up 2 dollars")
    }

    @Test func otherPlacesAreSpokenByRank() {
        #expect(
            BoardRow<EmptyView>.spoken(row(rank: 3, pnlMicros: -1_500_000), figure: .pnl)
                == "Rank 3, Ana, up 4.50 percent, down 1 dollar 50 cents")
    }

    @Test func aMissingReturnIsSaidSo() {
        #expect(
            BoardRow<EmptyView>.spoken(row(rank: 2, returnBps: nil, pnlMicros: 0), figure: .pnl)
                == "Rank 2, Ana, no return yet, no change")
    }

    @Test func aCabalSpeaksItsPot() {
        #expect(
            BoardRow<EmptyView>.spoken(row(rank: 2), figure: .value) == "Rank 2, Ana, up 4.50 percent, pot $50.00")
    }

    @Test func delayedPricesAreSaid() {
        #expect(
            BoardRow<EmptyView>.spoken(row(rank: 2, flags: [.stalePrices]), figure: .pnl)
                == "Rank 2, Ana, up 4.50 percent, up 2 dollars, prices delayed")
    }

    @Test func theViewerIsLabelledYou() {
        var viewer = row(rank: 2)
        viewer.isViewer = true
        #expect(
            BoardRow<EmptyView>.spoken(viewer, figure: .pnl) == "Rank 2, Ana, You, up 4.50 percent, up 2 dollars")
    }
}
