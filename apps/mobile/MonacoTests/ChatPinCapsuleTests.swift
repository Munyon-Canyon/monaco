import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ChatPinCapsuleTests {
    @Test func theSpokenLabelNamesThePinnerAndCountsOnlyWithManyPins() {
        let line = ChatPinLine(name: "Christelle", text: "5 of 8 to pass")

        #expect(
            ChatPinCapsule.spoken(line: line, counter: "1 of 2") == "Pinned by Christelle: 5 of 8 to pass. 1 of 2.")
        #expect(ChatPinCapsule.spoken(line: line, counter: "") == "Pinned by Christelle: 5 of 8 to pass.")
    }

    @Test func aPinWithoutAnAuthorNameReadsAsPlainPinnedText() {
        let line = ChatPinLine(name: "", text: "Ana joined")

        #expect(ChatPinCapsule.spoken(line: line, counter: "") == "Pinned: Ana joined.")
    }

    @Test func theCapsuleKeepsItsHeightAndEveryTargetKeepsTheTouchMinimum() {
        #expect(ChatPinMetrics.height == 40)
        #expect(ChatPinMetrics.hitHeight >= 44)
        #expect(ChatPinMetrics.fadeHeight == 46)
    }

    @Test func pinAndUnpinIconsResolveToSystemSymbols() {
        #expect(UIImage(systemName: MonacoIcon.pin.symbol) != nil)
        #expect(UIImage(systemName: MonacoIcon.unpin.symbol) != nil)
    }
}
