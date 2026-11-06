import Testing

@testable import Monaco
@testable import MonacoCore

@MainActor
struct ChatPinListSheetTests {
    @Test func twoVotesAndAPinShowBothSectionsAndTheMessagesHeader() {
        let layout = ChatPinListLayout(voteCount: 2, pinCount: 1)
        #expect(layout.showsVotes)
        #expect(layout.showsMessages)
        #expect(layout.showsMessagesHeader)
        #expect(!layout.isEmpty)
    }

    @Test func votesWithoutPinsShowOnlyTheVotesSection() {
        let layout = ChatPinListLayout(voteCount: 1, pinCount: 0)
        #expect(layout.showsVotes)
        #expect(!layout.showsMessages)
        #expect(!layout.showsMessagesHeader)
    }

    @Test func pinsWithoutVotesLookAsTheyDidBeforeWithNoHeader() {
        let layout = ChatPinListLayout(voteCount: 0, pinCount: 3)
        #expect(!layout.showsVotes)
        #expect(layout.showsMessages)
        #expect(!layout.showsMessagesHeader)
    }

    @Test func theSheetClosesWhenNothingIsLeft() {
        #expect(ChatPinListLayout(voteCount: 0, pinCount: 0).isEmpty)
    }

    @Test func theHeadersAndCountReadAsInTheFrame() {
        #expect(ChatOpenVotesModel.sectionTitle == "Open votes")
        #expect(ChatPinCopy.messagesTitle == "Messages")
        #expect(ChatPinCopy.sheetTitle == "Pinned")
        #expect(ChatPinListLayout.countLabel(2) == "2")
    }
}
