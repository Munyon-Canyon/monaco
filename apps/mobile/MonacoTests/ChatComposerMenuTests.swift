import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ChatComposerMenuTests {
    @Test func listsGifThesisAndStockInOrder() {
        #expect(ChatComposerMenu.items(hasThesis: true) == [.gif, .thesis, .stock])
    }

    @Test func hidesTheThesisItemWithoutAThesis() {
        #expect(ChatComposerMenu.items(hasThesis: false) == [.gif, .stock])
    }

    @Test func itemsCarryTheirIdentifiers() {
        let identifiers = ChatComposerItem.allCases.map(\.identifier)
        #expect(identifiers == ["chat-menu-gif", "chat-menu-thesis", "chat-menu-stock"])
    }

    @Test func titlesMatchTheCopy() {
        #expect(ChatComposerItem.allCases.map(\.title) == ["GIF", "Quote the thesis", "Share a stock"])
    }
}
