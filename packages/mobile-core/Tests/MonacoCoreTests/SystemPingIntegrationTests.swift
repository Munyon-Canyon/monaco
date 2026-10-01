import Foundation
import MonacoAPI
import MonacoCore
import OpenAPIURLSession
import XCTest

@available(macOS 14, iOS 17, *)
@MainActor
final class SystemPingIntegrationTests: XCTestCase {
    func testSendReachesEchoed() async throws {
        let env = ProcessInfo.processInfo.environment
        guard let base = env["MONACO_API_URL"], let serverURL = URL(string: base),
            let token = env["MONACO_DEV_TOKEN"], let user = env["MONACO_DEV_USER"],
            !base.isEmpty, !token.isEmpty, !user.isEmpty
        else {
            throw XCTSkip(
                "integration: set MONACO_API_URL, MONACO_DEV_TOKEN and MONACO_DEV_USER against just run backend"
            )
        }
        let stream = HintStream(
            serverURL: serverURL,
            transport: URLSessionTransport(),
            token: { token },
            refresh: { _ in nil },
            clock: ContinuousClock(),
            random: { Double.random(in: 0..<1) }
        )
        let model = SystemPingModel(
            api: APIClient(serverURL: serverURL, tokens: FixedToken(token: token)),
            hints: stream
        )
        await stream.start()
        let observe = Task { await model.observe() }
        defer {
            observe.cancel()
            Task { await stream.stop() }
        }
        let deadline = ContinuousClock.now.advanced(by: .seconds(3))
        await model.send(note: "hi")
        let echoed = await waitForEcho(model, until: deadline)
        let state = model.state
        XCTAssertTrue(echoed, "user \(user) ping was not echoed within 3 s; state \(state)")
    }

    private func waitForEcho(_ model: SystemPingModel, until deadline: ContinuousClock.Instant) async -> Bool {
        let clock = ContinuousClock()
        while clock.now < deadline {
            if case .loaded(let ping) = model.state, ping.echoed { return true }
            try? await clock.sleep(for: .milliseconds(50))
        }
        if case .loaded(let ping) = model.state { return ping.echoed }
        return false
    }
}

private struct FixedToken: MonacoAPI.AccessTokenProvider {
    let token: String

    func accessToken() async throws -> String? { token }

    func refreshedToken(replacing _: String) async throws -> String? { nil }
}
