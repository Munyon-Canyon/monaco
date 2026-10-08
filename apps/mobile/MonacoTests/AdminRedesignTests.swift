import Foundation
import MonacoCore
import Testing

@testable import Monaco

/// The rules on the Start a cabal screen: each choice carries a caption that says what it
/// means, in the words a member uses.
@MainActor
struct CabalRulesCopyTests {
    @Test func everyChoiceSaysWhatItMeansAndEachSaysSomethingDifferent() {
        let voterCaptions = CabalVoterMode.allCases.map { $0.caption }
        let thresholdCaptions = CabalThreshold.allCases.map { $0.caption }
        let expiryCaptions = CabalProposalExpiry.allCases.map { $0.caption }

        for captions in [voterCaptions, thresholdCaptions, expiryCaptions] {
            #expect(captions.allSatisfy { !$0.isEmpty })
            #expect(Set(captions).count == captions.count, "two choices of one rule read the same: \(captions)")
        }
    }

    @Test func theVoteWindowCaptionNamesItsOwnWindow() {
        for option in CabalProposalExpiry.allCases {
            #expect(option.caption.contains(option.label))
        }
    }

    /// The server still gets the values it validates; only the words changed.
    @Test func theRulesStillSendTheServersValues() {
        #expect(CabalVoterMode.everyone.rawValue == "all")
        #expect(CabalVoterMode.picked.rawValue == "list")
        #expect(CabalThreshold.majority.rawValue == "majority")
        #expect(CabalThreshold.unanimous.rawValue == "unanimous")
        #expect(CabalProposalExpiry.oneDay.rawValue == 86_400)
    }
}

/// Every string these screens add stays out of the plumbing vocabulary.
@MainActor
struct AdminCopyAuditTests {
    @Test func theNewCopyPassesTheMainFlowAudit() {
        let joinCopy = [
            JoinCabalCopy.title,
            JoinCabalModel.helper,
            JoinCabalModel.notFoundMessage,
            JoinCabalModel.malformedMessage,
            CabalEntry.requestedToast,
        ]

        #expect(MainFlowCopyAudit.stringsAreClean(CabalRulesCopy.auditedStrings))
        #expect(MainFlowCopyAudit.stringsAreClean(CabalDetailsCopy.auditedStrings))
        #expect(MainFlowCopyAudit.stringsAreClean(joinCopy))
    }
}
