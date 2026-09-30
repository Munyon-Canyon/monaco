import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif
import MonacoAPI
import XCTest

final class ToastCopyTests: XCTestCase {
    func testAProblemShowsTheServerMessage() {
        let problem = ProblemError(
            status: 404,
            code: .known(.notFound),
            message: "No such cabal.",
            traceID: "4bf92f3577b34da6a3ce929d0e0e4736",
            retryable: false
        )

        XCTAssertEqual(ToastCopy.message(for: .problem(problem)), "No such cabal.")
    }

    func testEveryOtherErrorHasFixedCopy() {
        XCTAssertEqual(ToastCopy.message(for: .transport(URLError(.notConnectedToInternet))), "You're offline. Try again.")
        XCTAssertEqual(ToastCopy.message(for: .inFlight), "Still working on it.")
        XCTAssertEqual(ToastCopy.message(for: .decoding("bad json")), "Something went wrong. Try again.")
        XCTAssertEqual(ToastCopy.message(for: .signedOut), "Please sign in again.")
        XCTAssertEqual(ToastCopy.message(for: .accountDeleted), "This account was deleted.")
    }
}
