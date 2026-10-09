import Testing

@testable import Monaco

struct CabalProposalsSummaryTests {
    @Test func joinsTheNonZeroPartsOnOneLine() {
        let text = CabalProposals.summary(needsVote: 2, open: 3, inProgress: 1, closed: 3)

        #expect(text == "2 to vote on · 1 trading · 3 closed recently")
    }

    @Test func saysOpenWhenEveryOpenProposalIsVoted() {
        let text = CabalProposals.summary(needsVote: 0, open: 2, inProgress: 0, closed: 0)

        #expect(text == "2 open")
    }
}
