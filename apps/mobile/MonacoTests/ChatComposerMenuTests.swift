import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ChatComposerMenuTests {
    @Test func listsGifPollThesisAndStockInOrder() {
        #expect(ChatComposerMenu.items(hasThesis: true) == [.gif, .poll, .thesis, .stock])
    }

    @Test func hidesTheThesisItemWithoutAThesis() {
        #expect(ChatComposerMenu.items(hasThesis: false) == [.gif, .poll, .stock])
    }

    @Test func hidesTheStockItemWithoutSearch() {
        #expect(ChatComposerMenu.items(hasThesis: false, hasStock: false) == [.gif, .poll])
    }

    @Test func itemsCarryTheirIdentifiers() {
        let identifiers = ChatComposerItem.allCases.map(\.identifier)
        #expect(identifiers == ["chat-menu-gif", "chat-menu-poll", "chat-menu-thesis", "chat-menu-stock"])
    }

    @Test func pollIsSecondWithThePollCheckIcon() {
        #expect(ChatComposerItem.allCases.map(\.rawValue).prefix(2) == ["gif", "poll"])
        #expect(ChatComposerItem.poll.icon == .pollCheck)
    }

    @Test func titlesMatchTheCopy() {
        #expect(ChatComposerItem.allCases.map(\.title) == ["GIF", "Poll", "Quote the thesis", "Share a stock"])
    }
}
