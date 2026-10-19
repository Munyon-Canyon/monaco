import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct CabalCoverCardTests {
    private func model(
        share: String? = "$7,000.00", ownership: String? = "35.00%",
        pnl: CabalCoverCardModel.Figure? = .init(amountText: "−$300.00", percentText: "−4.11%", direction: .loss),
        reason: String? = nil, votes: String? = "1 vote needed"
    ) -> CabalCoverCardModel {
        CabalCoverCardModel(
            id: "cabal-1", name: "Weekend Warriors", memberCountText: "8 members", votesNeededText: votes,
            shareText: share, ownershipText: ownership, pnl: pnl, pnlUnavailableReason: reason,
            groupReturn: .init(periodText: "1M", percentText: "−4.12%", direction: .loss), tint: .pine)
    }

    private func size(_ layout: CabalCoverCardLayout, text: DynamicTypeSize = .large) -> CGSize {
        let card = CabalCoverCard(model: model(), layout: layout, onOpen: {}, onShare: {})
            .environment(\.dynamicTypeSize, text)
        return UIHostingController(rootView: card).sizeThatFits(in: CGSize(width: 390, height: 2_000))
    }

    @Test func aFundedCardIsOneSentence() {
        #expect(
            model().spokenLabel
                == "Weekend Warriors, 8 members, 1 vote needed, your share $7,000.00, ownership 35.00%, "
                + "your all-time P&L down 300 dollars, down 4.11 percent, cabal 1 month down 4.12 percent")
    }

    @Test func aCardWithoutAShareSaysSo() {
        let label = model(share: nil, ownership: nil, pnl: nil, reason: "Not funded", votes: nil).spokenLabel
        #expect(
            label
                == "Weekend Warriors, 8 members, no share, your all-time P&L not funded, "
                + "cabal 1 month down 4.12 percent")
    }

    @Test func anUnavailablePnLIsSaidUnavailable() {
        let label = model(pnl: nil, reason: "Unavailable").spokenLabel
        #expect(label.contains("your all-time P&L unavailable"))
    }

    @Test func theRailHostsAtItsDesignSize() {
        let fitted = size(.rail)
        #expect(fitted.width == 300)
        #expect(fitted.height == 218)
    }

    @Test func theListIsAsTallAsItsDesignAtTheDefaultSize() {
        #expect(size(.list).height == 208)
    }

    @Test func largeTextGrowsTheCardWithoutClipping() {
        #expect(size(.rail, text: .accessibility3).height >= 330)
        #expect(size(.list, text: .accessibility3).height >= 260)
    }
}
