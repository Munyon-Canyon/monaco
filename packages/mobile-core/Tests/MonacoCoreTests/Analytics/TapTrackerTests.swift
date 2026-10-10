import MonacoAnalytics
import XCTest

final class TapTrackerTests: XCTestCase {
    private func taps(
        _ tracker: inout TapTracker,
        control: String = "buy",
        screen: String = "asset",
        interactive: Bool = true,
        at offsets: [Duration]
    ) -> [TapSignal?] {
        offsets.map { tracker.record(controlID: control, screen: screen, interactive: interactive, at: $0) }
    }

    func testRageOnTheFourthTapWithinOneSecond() {
        var tracker = TapTracker()
        let signals = taps(&tracker, at: [.zero, .milliseconds(200), .milliseconds(400), .milliseconds(600)])
        XCTAssertEqual(signals, [nil, nil, nil, .rage(controlID: "buy", screen: "asset")])
    }

    func testNoRageWhenFourTapsSpanLongerThanOneSecond() {
        var tracker = TapTracker()
        let signals = taps(&tracker, at: [.zero, .milliseconds(400), .milliseconds(800), .milliseconds(1200)])
        XCTAssertEqual(signals, [nil, nil, nil, nil])
    }

    func testOneSignalPerBurst() {
        var tracker = TapTracker()
        let offsets = (0..<10).map { Duration.milliseconds($0 * 150) }
        let signals = taps(&tracker, at: offsets).compactMap { $0 }
        XCTAssertEqual(signals, [.rage(controlID: "buy", screen: "asset")])
    }

    func testANewBurstAfterAPauseSignalsAgain() {
        var tracker = TapTracker()
        let early = taps(&tracker, at: [.zero, .milliseconds(100), .milliseconds(200), .milliseconds(300)])
        let late = taps(&tracker, at: [.seconds(5), .milliseconds(5100), .milliseconds(5200), .milliseconds(5300)])
        XCTAssertEqual(early.compactMap { $0 }.count, 1)
        XCTAssertEqual(late.compactMap { $0 }.count, 1)
    }

    func testTapsOnDifferentControlsDoNotAdd() {
        var tracker = TapTracker()
        var signals: [TapSignal?] = []
        for (index, control) in ["a", "b", "a", "b", "a", "b"].enumerated() {
            signals.append(
                tracker.record(
                    controlID: control, screen: "asset", interactive: true, at: .milliseconds(index * 100)
                )
            )
        }
        XCTAssertEqual(signals.compactMap { $0 }, [])
    }

    func testSameIdentifierOnAnotherScreenIsAnotherControl() {
        var tracker = TapTracker()
        _ = taps(&tracker, screen: "home", at: [.zero, .milliseconds(100), .milliseconds(200)])
        let signals = taps(&tracker, screen: "asset", at: [.milliseconds(300)])
        XCTAssertEqual(signals, [nil])
    }

    func testDeadTapOnTheSecondTapOfANonInteractiveElement() {
        var tracker = TapTracker()
        let signals = taps(
            &tracker, control: "home_title", screen: "home", interactive: false,
            at: [.zero, .milliseconds(300), .milliseconds(500), .milliseconds(700), .milliseconds(900)]
        )
        XCTAssertEqual(signals, [nil, .dead(screen: "home", controlID: "home_title"), nil, nil, nil])
    }

    func testNoDeadTapWhenTheSecondTapComesAfterOneSecond() {
        var tracker = TapTracker()
        let signals = taps(&tracker, interactive: false, at: [.zero, .milliseconds(1100)])
        XCTAssertEqual(signals, [nil, nil])
    }

    func testTapsWithoutAnIdentifierAreIgnored() {
        var tracker = TapTracker()
        let signals = taps(&tracker, control: "", interactive: false, at: [.zero, .milliseconds(100)])
        XCTAssertEqual(signals, [nil, nil])
    }
}
