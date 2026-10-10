import Foundation
import MonacoAnalytics
import XCTest

final class FlowAttemptTests: XCTestCase {
    private let first = UUID(uuidString: "11111111-1111-4111-8111-111111111111") ?? UUID()
    private let second = UUID(uuidString: "22222222-2222-4222-8222-222222222222") ?? UUID()

    func testStartMintsALowercaseHyphenatedID() {
        var attempts = FlowAttempt()
        let id = attempts.start(.onboarding, id: first)
        XCTAssertEqual(id, "11111111-1111-4111-8111-111111111111")
        XCTAssertEqual(attempts.flowID(for: .onboarding), id)
    }

    func testRandomIDsAreLowercase() {
        var attempts = FlowAttempt()
        let id = attempts.start(.vote)
        XCTAssertEqual(id, id.lowercased())
        XCTAssertNotNil(UUID(uuidString: id))
    }

    func testNewStartReplacesTheAttempt() {
        var attempts = FlowAttempt()
        attempts.start(.onboarding, id: first)
        attempts.start(.onboarding, id: second)
        XCTAssertEqual(attempts.flowID(for: .onboarding), FlowAttempt.format(second))
    }

    func testFlowsKeepSeparateAttempts() {
        var attempts = FlowAttempt()
        attempts.start(.onboarding, id: first)
        attempts.start(.propose, id: second)
        XCTAssertEqual(attempts.flowID(for: .onboarding), FlowAttempt.format(first))
        XCTAssertEqual(attempts.flowID(for: .propose), FlowAttempt.format(second))
        XCTAssertNil(attempts.flowID(for: .vote))
    }
}
