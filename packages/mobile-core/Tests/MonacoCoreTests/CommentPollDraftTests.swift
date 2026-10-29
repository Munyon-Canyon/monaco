import XCTest

@testable import MonacoCore

final class CommentPollDraftTests: XCTestCase {
    func testAnEmptyDraftIsNotValidAndStartsAtTwoOptionsAnd24Hours() {
        let draft = CommentPollDraft()
        XCTAssertFalse(draft.isValid)
        XCTAssertEqual(draft.options.count, 2)
        XCTAssertEqual(draft.closes, .day)
        XCTAssertEqual(CommentPollDraft.CloseChoice.allCases.map(\.label), ["1h", "24h", "3d", "7d"])
    }

    func testAFilledDraftBuildsAPollInput() {
        let draft = CommentPollDraft(question: " Trim? ", options: ["Yes ", "No"], closes: .week)
        guard case .poll(let input) = draft.input else { return XCTFail("no poll input") }
        XCTAssertEqual(input.question, "Trim?")
        XCTAssertEqual(input.options, ["Yes", "No"])
        XCTAssertEqual(input.closesInHours, 168)
    }

    func testTheQuestionAndOptionBoundsApply() {
        let long = String(repeating: "a", count: 101)
        XCTAssertFalse(CommentPollDraft(question: long, options: ["a", "b"]).isValid)
        XCTAssertFalse(CommentPollDraft(question: "Q", options: [String(repeating: "a", count: 41), "b"]).isValid)
        XCTAssertFalse(CommentPollDraft(question: "Q", options: ["a", " "]).isValid)
        XCTAssertTrue(CommentPollDraft(question: String(repeating: "a", count: 100), options: ["a", "b"]).isValid)
    }

    func testOptionsAreDistinctIgnoringCaseAndCappedAtFour() {
        XCTAssertFalse(CommentPollDraft(question: "Q", options: ["Yes", "yes"]).isValid)
        XCTAssertFalse(CommentPollDraft(question: "Q", options: ["a"]).isValid)
        var draft = CommentPollDraft(question: "Q", options: ["a", "b", "c", "d"])
        XCTAssertFalse(draft.canAddOption)
        draft.addOption()
        XCTAssertEqual(draft.options.count, 4)
        var three = CommentPollDraft(question: "Q", options: ["a", "b"])
        three.addOption()
        XCTAssertEqual(three.options.count, 3)
        XCTAssertFalse(three.isValid)
    }

    func testTheSheetTitleFollowsTheThread() {
        XCTAssertEqual(CommentsCopy.pollTitle(isCabal: true), "Ask the cabal")
        XCTAssertEqual(CommentsCopy.pollTitle(isCabal: false), "Ask a question")
    }
}
