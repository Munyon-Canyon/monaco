import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct EditCabalCopyTests {
    @Test func theEditScreenSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean(EditCabalCopy.auditedStrings))
        #expect(EditCabalCopy.rulesFooter == "Rule changes apply to new proposals. Open votes keep their rules.")
        #expect(EditCabalCopy.saved == "Cabal updated.")
        #expect(EditCabalCopy.votersRow == "Voters")
        #expect(EditCabalCopy.screenTitle == "Cabal settings")
    }

    @Test func theVoterPickerSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean(CabalVotersCopy.auditedStrings))
        #expect(CabalVotersCopy.saved == "Voters updated.")
        #expect(CabalVotersCopy.alwaysVotes == "Always votes")
    }

    @Test func theRulesSectionSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean(CabalRulesSlotCopy.auditedStrings))
        #expect(CabalRulesSlotCopy.failed == "Couldn't load the rules.")
    }
}
