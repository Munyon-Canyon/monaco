import MonacoCore
import MonacoTestClock
import XCTest

final class ResendCooldownTests: XCTestCase {
    func testBeforeAnySendTheResendIsOpen() {
        let cooldown = ResendCooldown(clock: TestClock())

        XCTAssertTrue(cooldown.canResend)
        XCTAssertEqual(cooldown.label, "Send a new code")
    }

    func testTheResendWaitsThirtySecondsAfterASend() {
        let clock = TestClock()
        var cooldown = ResendCooldown(clock: clock)

        cooldown.restart()
        XCTAssertFalse(cooldown.canResend)
        XCTAssertEqual(cooldown.label, "Send a new code in 30s")

        clock.advance(by: .seconds(29))
        XCTAssertFalse(cooldown.canResend)
        XCTAssertEqual(cooldown.label, "Send a new code in 1s")

        clock.advance(by: .seconds(1))
        XCTAssertTrue(cooldown.canResend)
        XCTAssertEqual(cooldown.label, "Send a new code")
    }

    func testAPartSecondLeftRoundsUp() {
        let clock = TestClock()
        var cooldown = ResendCooldown(clock: clock)

        cooldown.restart()
        clock.advance(by: .milliseconds(29_500))

        XCTAssertEqual(cooldown.secondsLeft, 1)
    }

    func testAResendRestartsTheThirtySeconds() {
        let clock = TestClock()
        var cooldown = ResendCooldown(clock: clock)
        cooldown.restart()
        clock.advance(by: .seconds(30))

        cooldown.restart()

        XCTAssertEqual(cooldown.label, "Send a new code in 30s")
        clock.advance(by: .seconds(29))
        XCTAssertFalse(cooldown.canResend)
        clock.advance(by: .seconds(1))
        XCTAssertTrue(cooldown.canResend)
    }
}
