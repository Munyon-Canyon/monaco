import MonacoAPI
import XCTest

@testable import MonacoTestClock

final class TestClockTests: XCTestCase {
    func testAdvanceResumesSleepersInDeadlineOrder() async throws {
        let clock = TestClock()
        let sleeps = [
            Task { try await clock.sleep(for: .milliseconds(30)) },
            Task { try await clock.sleep(for: .milliseconds(10)) },
            Task { try await clock.sleep(for: .milliseconds(20)) },
        ]
        let parked = await clock.state.until { $0.pending == 3 }
        XCTAssertTrue(parked)

        let started = ContinuousClock.now
        clock.advance(by: .milliseconds(30))
        let elapsed = started.duration(to: .now)
        XCTAssertEqual(
            clock.state.current.resumed,
            [.milliseconds(10), .milliseconds(20), .milliseconds(30)]
        )
        XCTAssertEqual(clock.state.current.pending, 0)
        XCTAssertLessThan(elapsed, .milliseconds(50))
        for sleep in sleeps {
            try await sleep.value
        }
    }

    func testTiedDeadlinesResumeInInsertionOrderNotHashOrder() {
        let tied = TestClock.Instant(offset: .milliseconds(10))
        let earlier = TestClock.Instant(offset: .milliseconds(5))
        let scrambled = [
            (id: 2, deadline: tied),
            (id: 0, deadline: earlier),
            (id: 3, deadline: tied),
            (id: 1, deadline: tied),
        ]
        XCTAssertEqual(TestClock.orderedIDs(scrambled), [0, 1, 2, 3])
    }

    func testAdvanceResumesTiedSleepersInRegistrationOrder() async throws {
        let clock = TestClock()
        var sleeps: [Task<Void, Error>] = []
        for count in 1...4 {
            sleeps.append(Task { try await clock.sleep(for: .milliseconds(10)) })
            let parked = await clock.state.until { $0.pending == count }
            XCTAssertTrue(parked)
        }
        let started = ContinuousClock.now
        clock.advance(by: .milliseconds(10))
        XCTAssertEqual(clock.state.current.resumeOrder, [1, 2, 3, 4])
        XCTAssertLessThan(started.duration(to: .now), .milliseconds(50))
        for sleep in sleeps {
            try await sleep.value
        }
    }

    func testPingSamples() {
        XCTAssertFalse(Components.Schemas.Ping.sample.echoed)
        XCTAssertTrue(Components.Schemas.Ping.sampleEchoed.echoed)
        XCTAssertEqual(Components.Schemas.Ping.sample.id, Components.Schemas.Ping.sampleEchoed.id)
        XCTAssertEqual(Components.Schemas.Ping.sample.note, "hi")
    }
}
