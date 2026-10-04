import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct CabalChatViewTests {
    @Test func aMemberSeesTheComposerComingSoon() {
        #expect(CabalChatView.composer(for: .sample(role: "member")) == .comingSoon)
    }

    @Test func aNonMemberSeesTheChatClosed() {
        #expect(CabalChatView.composer(for: .sample(role: nil)) == .closed)
    }
}
