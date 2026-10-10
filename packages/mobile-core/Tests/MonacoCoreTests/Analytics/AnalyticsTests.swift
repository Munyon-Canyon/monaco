import Foundation
import MonacoAnalytics
import Synchronization
import XCTest

final class RecordingSink: AnalyticsSink {
    enum Call: Equatable {
        case identify(String, [String: AnalyticsValue])
        case capture(String, [String: AnalyticsValue])
        case screen(String)
        case reset
    }

    private let recorded = Mutex<[Call]>([])

    var calls: [Call] { recorded.withLock { $0 } }

    func identify(userID: String, properties: [String: AnalyticsValue]) {
        recorded.withLock { $0.append(.identify(userID, properties)) }
    }

    func capture(_ name: String, properties: [String: AnalyticsValue]) {
        recorded.withLock { $0.append(.capture(name, properties)) }
    }

    func screen(_ name: String) {
        recorded.withLock { $0.append(.screen(name)) }
    }

    func reset() {
        recorded.withLock { $0.append(.reset) }
    }
}

@MainActor
final class AnalyticsTests: XCTestCase {
    private let sink = RecordingSink()
    private lazy var analytics = Analytics(sink: sink)
    private let attempt = UUID(uuidString: "33333333-3333-4333-8333-333333333333") ?? UUID()

    func testIdentifyLowercasesTheUserIDAndScrubsProperties() {
        analytics.identify(
            userID: "7B0C2F4E-9A1D-4C7B-8E5F-0A1B2C3D4E5F",
            properties: ["auth_state": "CREATED", "email": "ada@example.com", "login_provider": "google"]
        )
        XCTAssertEqual(
            sink.calls,
            [
                .identify(
                    "7b0c2f4e-9a1d-4c7b-8e5f-0a1b2c3d4e5f",
                    ["auth_state": "CREATED", "login_provider": "google"]
                )
            ]
        )
    }

    func testCaptureScrubsPII() {
        let address = AnalyticsValue.string(String(repeating: "C", count: 44))
        analytics.capture("custom", properties: ["wallet": "x", "to": address, "ok": 1])
        XCTAssertEqual(sink.calls, [.capture("custom", ["ok": 1])])
    }

    func testStepsInOneAttemptShareAFlowID() {
        analytics.start(.onboarding, id: attempt)
        analytics.step(.onboarding(.loginStarted))
        analytics.step(.onboarding(.loginCompleted), properties: ["login_provider": "google", "phone": "+14155550123"])
        let flowID: AnalyticsValue = "33333333-3333-4333-8333-333333333333"
        XCTAssertEqual(
            sink.calls,
            [
                .capture("onboarding_login_started", ["flow_id": flowID]),
                .capture("onboarding_login_completed", ["flow_id": flowID, "login_provider": "google"]),
            ]
        )
    }

    func testStepWithoutStartBeginsAnAttemptAndReusesIt() {
        analytics.step(.vote(.proposalViewed))
        analytics.step(.vote(.voteCast))
        let ids = sink.calls.compactMap { call -> AnalyticsValue? in
            guard case .capture(_, let properties) = call else { return nil }
            return properties["flow_id"]
        }
        XCTAssertEqual(ids.count, 2)
        XCTAssertEqual(ids[0], ids[1])
    }

    func testRestartingAFlowChangesTheFlowID() {
        analytics.start(.propose, id: attempt)
        analytics.step(.propose(.proposeOpened))
        analytics.start(.propose, id: UUID(uuidString: "44444444-4444-4444-8444-444444444444") ?? UUID())
        analytics.step(.propose(.proposeOpened))
        let ids = sink.calls.compactMap { call -> AnalyticsValue? in
            guard case .capture(_, let properties) = call else { return nil }
            return properties["flow_id"]
        }
        XCTAssertNotEqual(ids[0], ids[1])
    }

    func testTapSignalsBecomeEvents() {
        for index in 0..<4 {
            analytics.tap(controlID: "buy", screen: "asset", interactive: true, at: .milliseconds(index * 100))
        }
        analytics.tap(controlID: "title", screen: "home", interactive: false, at: .seconds(10))
        analytics.tap(controlID: "title", screen: "home", interactive: false, at: .milliseconds(10_200))
        XCTAssertEqual(
            sink.calls,
            [
                .capture("rage_tap", ["control_id": "buy", "screen": "asset"]),
                .capture("dead_tap", ["control_id": "title", "screen": "home"]),
            ]
        )
    }

    func testScreenPassesThroughAndIsRemembered() {
        analytics.screen("login")
        XCTAssertEqual(sink.calls, [.screen("login")])
        XCTAssertEqual(analytics.currentScreen, "login")
    }

    func testHasAttemptFollowsStartAndReset() {
        XCTAssertFalse(analytics.hasAttempt(for: .onboarding))
        analytics.start(.onboarding, id: attempt)
        XCTAssertTrue(analytics.hasAttempt(for: .onboarding))
        XCTAssertFalse(analytics.hasAttempt(for: .vote))
        analytics.reset()
        XCTAssertFalse(analytics.hasAttempt(for: .onboarding))
        XCTAssertEqual(analytics.currentScreen, "")
    }

    func testResetClearsAttemptsAndResetsTheSink() {
        analytics.start(.onboarding, id: attempt)
        analytics.reset()
        analytics.step(.onboarding(.loginStarted))
        guard case .capture(_, let properties) = sink.calls.last else { return XCTFail("no capture") }
        XCTAssertNotEqual(properties["flow_id"], "33333333-3333-4333-8333-333333333333")
        XCTAssertEqual(sink.calls.first, .reset)
    }
}
