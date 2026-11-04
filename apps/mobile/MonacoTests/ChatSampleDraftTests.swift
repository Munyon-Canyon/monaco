import SwiftUI
import Testing

@testable import Monaco

struct ChatSampleDraftTests {
    #if DEBUG
    @Test func theNvdaPresetSeedsTheNvdaQuestion() {
        let draft = ChatComposerSeed.draft(arguments: [ChatSampleDraft.flag, "nvda"], isDebugBuild: true)

        #expect(draft == "Should we buy more NVDA?")
    }

    @Test func aLiteralValueSeedsItself() {
        let draft = ChatComposerSeed.draft(arguments: [ChatSampleDraft.flag, "hello there"], isDebugBuild: true)

        #expect(draft == "hello there")
    }

    @Test func noFlagSeedsAnEmptyDraft() {
        #expect(ChatComposerSeed.draft(arguments: ["Monaco"], isDebugBuild: true) == "")
    }

    @Test func aFlagWithoutAValueSeedsAnEmptyDraft() {
        #expect(ChatComposerSeed.draft(arguments: [ChatSampleDraft.flag], isDebugBuild: true) == "")
    }

    @Test func theArgumentDoesNothingInRelease() {
        #expect(ChatComposerSeed.draft(arguments: [ChatSampleDraft.flag, "nvda"], isDebugBuild: false) == "")
    }
    #else
    @Test func theArgumentDoesNothingInRelease() {
        #expect(ChatComposerSeed.draft(arguments: ["-MonacoChatSampleDraft", "nvda"], isDebugBuild: false) == "")
    }
    #endif
}
