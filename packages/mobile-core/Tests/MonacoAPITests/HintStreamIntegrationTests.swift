import Foundation
import OpenAPIRuntime
import OpenAPIURLSession
import XCTest

@testable import MonacoAPI

final class HintStreamIntegrationTests: XCTestCase {
    func testPingEchoArrivesAsAUserHint() async throws {
        let env = ProcessInfo.processInfo.environment
        guard let base = env["MONACO_API_URL"], let serverURL = URL(string: base),
            let token = env["MONACO_DEV_TOKEN"], let user = env["MONACO_DEV_USER"]
        else {
            throw XCTSkip(
                "integration: set MONACO_API_URL, MONACO_DEV_TOKEN and MONACO_DEV_USER against just run backend")
        }
        let stream = HintStream(
            serverURL: serverURL,
            transport: URLSessionTransport(),
            token: { token },
            refresh: { _ in nil },
            clock: ContinuousClock(),
            random: { Double.random(in: 0..<1) }
        )
        let hints = Collector(stream.hints(matching: .user(what: "ping_echoed")))
        await stream.start()
        defer { Task { await stream.stop() } }
        _ = await hints.first(1)

        let client = Client(
            serverURL: serverURL,
            transport: URLSessionTransport(),
            middlewares: [HeadersMiddleware(accessToken: { token }), ProblemMiddleware()]
        )
        let started = ContinuousClock.now
        _ = try await client.postSystemPing(
            headers: .init(idempotencyKey: UUID().uuidString),
            body: .json(.init(note: "HintStreamIntegrationTests"))
        ).created
        let arrived = await hints.hints.until(within: .seconds(2)) { received in
            received.contains { hint in
                if case .changed(.user(user), what: "ping_echoed", _) = hint { return true }
                return false
            }
        }

        XCTAssertTrue(arrived, "no ping_echoed hint for user:\(user) within 2 s; got \(hints.hints.current)")
        XCTAssertLessThan(ContinuousClock.now - started, .seconds(2))
    }
}
