import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct EditCabalCopyTests {
    @Test func theEditScreenSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean(EditCabalCopy.auditedStrings))
        #expect(EditCabalCopy.rulesFooter == "Rule changes apply to new proposals. Open votes keep their rules.")
        #expect(EditCabalCopy.saved == "Cabal updated.")
        #expect(EditCabalCopy.votersHeader == "Voters")
        #expect(EditCabalCopy.alwaysVotes == "Always votes")
        #expect(EditCabalCopy.screenTitle == "Cabal settings")
    }

    @Test func theApproveRowVoteToggleSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean([CabalJoinCopy.canVote]))
        #expect(CabalJoinCopy.canVote == "Can vote")
    }

    @Test func theRulesSectionSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean(CabalRulesSlotCopy.auditedStrings))
        #expect(CabalRulesSlotCopy.failedThing == "the rules")
    }
}
