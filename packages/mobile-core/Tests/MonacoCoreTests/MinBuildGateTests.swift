import XCTest

@testable import MonacoCore

@MainActor
final class MinBuildGateTests: XCTestCase {
    private let store = URL(string: "https://apps.apple.com/app/id1")!

    private func gate(build: Int, min: Int) -> MinBuildGate {
        let store = store
        return MinBuildGate(currentBuild: build) { (min, store) }
    }

    func testOkWhenBuildIsAboveMinimum() async throws {
        let gate = gate(build: 5, min: 3)
        await gate.refresh()
        XCTAssertEqual(gate.state, .ok)
    }

    func testOkWhenBuildEqualsMinimum() async throws {
        let gate = gate(build: 5, min: 5)
        await gate.refresh()
        XCTAssertEqual(gate.state, .ok)
    }

    func testBlockedWhenMinimumIsAboveBuild() async throws {
        let gate = gate(build: 4, min: 5)
        await gate.refresh()
        XCTAssertEqual(gate.state, .blocked(store))
    }

    func testOkWhenMinimumIsZero() async throws {
        let gate = gate(build: 0, min: 0)
        await gate.refresh()
        XCTAssertEqual(gate.state, .ok)
    }

    func testOkOnFetchError() async throws {
        struct Boom: Error {}
        let gate = MinBuildGate(currentBuild: 1) { throw Boom() }
        await gate.refresh()
        XCTAssertEqual(gate.state, .ok)
    }
}
