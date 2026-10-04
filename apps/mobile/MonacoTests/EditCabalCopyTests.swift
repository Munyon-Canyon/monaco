import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct EditCabalCopyTests {
    @Test func theEditScreenSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean(EditCabalCopy.auditedStrings))
        #expect(EditCabalCopy.rulesFooter == "Rule changes apply to new proposals. Open votes keep their rules.")
        #expect(EditCabalCopy.saved == "Cabal updated.")
    }

    @Test func theRulesSectionSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean(CabalRulesSlotCopy.auditedStrings))
        #expect(CabalRulesSlotCopy.failed == "Couldn't load the rules.")
    }
}
