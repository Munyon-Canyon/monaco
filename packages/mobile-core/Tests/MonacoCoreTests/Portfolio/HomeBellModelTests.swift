import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class HomeBellModelTests: XCTestCase {
    private let countPath = "/v1/me/catch-up/count"

    private func make(_ counts: [Int], hints: FakeHintStream = FakeHintStream()) -> (HomeBellModel, StubTransport) {
        let transport = StubTransport(routes: [countPath: counts.map { .json(.ok, #"{"count":\#($0)}"#) }])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return (HomeBellModel(api: api, hints: hints), transport)
    }

    func testZeroShowsNoBadge() async {
        let (model, _) = make([0])
        await model.load()
        XCTAssertNil(model.badgeText)
        XCTAssertEqual(model.accessibilityLabel, "Catch up")
    }

    func testSixShowsSixAndVoiceOverReadsIt() async {
        let (model, _) = make([6])
        await model.load()
        XCTAssertEqual(model.badgeText, "6")
        XCTAssertEqual(model.accessibilityLabel, "Catch up, 6 new")
    }

    func testNineShowsNineAndTwelveShowsNinePlus() async {
        let (nine, _) = make([9])
        await nine.load()
        XCTAssertEqual(nine.badgeText, "9")
        let (twelve, _) = make([12])
        await twelve.load()
        XCTAssertEqual(twelve.badgeText, "9+")
        XCTAssertEqual(twelve.accessibilityLabel, "Catch up, 12 new")
    }

    func testAHintReloadsTheCount() async {
        let hints = FakeHintStream()
        let (model, transport) = make([6, 7], hints: hints)
        await model.load()
        let observer = Task { await model.observe() }
        await hints.waitForSubscribers(1)
        model.setVisible(true)
        await hints.send(.changed(.user("me"), what: "catch_up_changed", id: "1"))
        for _ in 0..<5_000 where model.count != 7 { await Task.yield() }
        observer.cancel()
        XCTAssertEqual(model.count, 7)
        let sent = await transport.sent.count
        XCTAssertEqual(sent, 2)
    }

    func testAFailedReloadKeepsTheLastCount() async {
        let transport = StubTransport(routes: [
            countPath: [.json(.ok, #"{"count":6}"#), .json(.internalServerError, "{}")]
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let model = HomeBellModel(api: api, hints: FakeHintStream())
        await model.load()
        await model.load()
        XCTAssertEqual(model.count, 6)
    }
}
