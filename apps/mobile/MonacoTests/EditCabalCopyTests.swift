import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct EditCabalCopyTests {
    @Test func theEditScreenSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean(EditCabalCopy.auditedStrings))
        #expect(EditCabalCopy.saved == "Cabal updated.")
        #expect(EditCabalCopy.votersHeader == "Voters")
        #expect(EditCabalCopy.alwaysVotes == "Always votes")
        #expect(EditCabalCopy.screenTitle == "Cabal settings")
    }

    @Test func saveStaysOffForNamesTheServerRefuses() {
        #expect(CreateCabalForm(name: "ab").nameProblem != nil)
        #expect(CreateCabalForm(name: String(repeating: "a", count: 41)).nameProblem != nil)
        #expect(CreateCabalForm(name: "abc").nameProblem == nil)
        #expect(CreateCabalForm(name: String(repeating: "a", count: 40)).nameProblem == nil)
    }

    @Test func theApproveRowVoteToggleSpeaksTheProductLanguage() {
        #expect(MainFlowCopyAudit.stringsAreClean([CabalJoinCopy.canVote]))
        #expect(CabalJoinCopy.canVote == "Can vote")
    }

    @Test func theRulesSectionSpeaksTheProductLanguage() {
        #expect(CabalRulesTabCopy.failedThing == "the rules")
    }
}
