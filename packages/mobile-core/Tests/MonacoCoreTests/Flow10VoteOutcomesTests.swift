import MonacoCore
import MonacoFlows
import XCTest

final class Flow10VoteOutcomesTests: XCTestCase {
    private let message = "Server says no."

    func testEveryFlow10CodeIsMappedExplicitly() {
        let expected: [Flow10Outcome: VoteRefusal] = [
            .unauthorized: .message(message),
            .proposalNotFound: .message(message),
            .notAVoter: .notAVoter(message: message),
            .proposalClosed: .closed(message: message),
        ]
        for outcome in Flow10Outcome.allCases {
            guard let code = outcome.code else { continue }
            XCTAssertEqual(VoteRefusal(flowProblem(code)), expected[outcome], code)
        }
    }

    func testEveryFlow10aCodeIsMappedExplicitly() {
        let expected: [Flow10aOutcome: UndoRefusal] = [
            .unauthorized: .message(message),
            .proposalNotFound: .message(message),
            .notAVoter: .message(message),
            .proposalClosed: .closed(message: message),
            .voteNotFound: .voteNotFound(message: message),
            .undoExpired: .expired(message: message),
        ]
        for outcome in Flow10aOutcome.allCases {
            guard let code = outcome.code else { continue }
            XCTAssertEqual(UndoRefusal(flowProblem(code)), expected[outcome], code)
        }
    }

    func testOnlyClosedAndNotAVoterReloadAfterAVote() {
        XCTAssertTrue(VoteRefusal(flowProblem("proposal_closed")).reloads)
        XCTAssertTrue(VoteRefusal(flowProblem("not_a_voter")).reloads)
        XCTAssertFalse(VoteRefusal(flowProblem("unauthorized")).reloads)
        XCTAssertFalse(VoteRefusal(.transport(URLError(.networkConnectionLost))).reloads)
    }

    func testAnExpiredUndoKeepsTheVoteAndTheOthersReload() {
        XCTAssertFalse(UndoRefusal(flowProblem("undo_expired")).reloads)
        XCTAssertTrue(UndoRefusal(flowProblem("vote_not_found")).reloads)
        XCTAssertTrue(UndoRefusal(flowProblem("proposal_closed")).reloads)
    }

    func testARequestThatNeverAnsweredToastsTheOfflineMessage() {
        XCTAssertEqual(
            VoteRefusal(.transport(URLError(.networkConnectionLost))), .message("You're offline. Try again."))
        XCTAssertEqual(UndoRefusal(.signedOut).message, "Please sign in again.")
        XCTAssertEqual(VoteRefusal(.signedOut), .message("Please sign in again."))
    }
}
