import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ProposalResultCardTests {
    private func size(_ status: ProposalStatus, caption: String?) -> CGSize {
        let card = ProposalResultCard(
            status: status, tally: ProposalTally(yes: 5, no: 1, voters: 8, needed: 5), caption: caption,
            accessibilitySummary: "Result"
        ) {
            EmptyView()
        }
        let host = UIHostingController(rootView: card)
        return host.sizeThatFits(in: CGSize(width: 390, height: 2_000))
    }

    @Test(arguments: [ProposalStatus.passed, .executed, .failed, .expired, .withdrawn, .voided])
    func everyClosedStatusDrawsACard(status: ProposalStatus) {
        #expect(size(status, caption: "filled at $219.40 · Oct 22").height > 0)
    }

    @Test func theCaptionAddsALine() {
        #expect(size(.passed, caption: "needed 3 · no trade").height > size(.passed, caption: nil).height)
    }
}
