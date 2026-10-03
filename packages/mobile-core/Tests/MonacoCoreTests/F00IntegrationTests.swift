import Foundation
import MonacoAPI
import MonacoCore
import MonacoFlows
import OpenAPIURLSession
import XCTest

#if canImport(FoundationNetworking)
import FoundationNetworking
import OpenAPIAsyncHTTPClient

private func streamTransport() -> any ClientTransport { AsyncHTTPClientTransport() }
#else
private func streamTransport() -> any ClientTransport { URLSessionTransport() }
#endif

@MainActor
final class F00IntegrationTests: XCTestCase {
    func test_F00_RecordPing_ok() async throws {
        let (serverURL, seeded) = try seed("ok")
        XCTAssertFalse(seeded.token.isEmpty)
        let stream = HintStream(
            serverURL: serverURL,
            transport: streamTransport(),
            token: { seeded.token },
            refresh: { _ in nil },
            clock: ContinuousClock(),
            random: { Double.random(in: 0..<1) }
        )
        let model = SystemPingModel(
            api: APIClient(serverURL: serverURL, tokens: SeededToken(token: seeded.token)),
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
        XCTAssertTrue(echoed, "user \(seeded.userID) ping was not echoed within 3 s; state \(state)")
    }

    func test_F00_RecordPing_InvalidInput() async throws {
        let (serverURL, seeded) = try seed("InvalidInput")
        let model = pingModel(serverURL, token: seeded.token)

        await model.send(note: String(repeating: "a", count: 141))

        let state = model.state
        guard case .invalidInput = state else {
            XCTFail("expected the invalid input outcome, got \(state)")
            return
        }
    }

    func test_F00_RecordPing_Unauthorized() async throws {
        let (serverURL, seeded) = try seed("Unauthorized")
        XCTAssertEqual(seeded.token, "")
        let model = pingModel(serverURL, token: seeded.token)

        await model.send(note: "hi")

        XCTAssertEqual(model.state, .unauthorized)
    }

    func test_F00_RecordPing_interrupted() async throws {
        let (serverURL, seeded) = try seed("crash:after-publish")
        let transport = LoseFirstPost()
        let model = pingModel(serverURL, token: seeded.token, transport: transport)

        await model.send(note: "hi")
        let lost = model.state
        guard case .failed(.transport) = lost else {
            XCTFail("expected the lost response to fail as a transport error, got \(lost)")
            return
        }
        await model.send(note: "hi")

        let state = model.state
        guard case .loaded(let ping) = state else {
            XCTFail("expected the retry to replay the recorded ping, got \(state)")
            return
        }
        let recorded = await transport.recordedID
        let posts = await transport.posts
        XCTAssertEqual(posts, 2)
        XCTAssertEqual(ping.id, recorded)
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

    private func pingModel(
        _ serverURL: URL,
        token: String,
        transport: any ClientTransport = URLSessionTransport()
    ) -> SystemPingModel {
        SystemPingModel(
            api: APIClient(serverURL: serverURL, tokens: SeededToken(token: token), transport: transport),
            hints: NoHints()
        )
    }

    private func seed(_ outcome: String) throws -> (URL, SeedResult) {
        let env = ProcessInfo.processInfo.environment
        guard let base = env["MONACO_API_URL"], let serverURL = URL(string: base), !base.isEmpty,
            let bin = env["MONACO_SEED_BIN"], let dir = env["MONACO_SEED_DIR"]
        else {
            throw XCTSkip("integration: run scripts/ci/mobile-integration.sh, which sets MONACO_SEED_BIN")
        }
        let process = Process()
        process.executableURL = URL(fileURLWithPath: bin)
        process.arguments = ["flows", "seed", "00", outcome]
        process.currentDirectoryURL = URL(fileURLWithPath: dir)
        let out = Pipe()
        let err = Pipe()
        process.standardOutput = out
        process.standardError = err
        try process.run()
        let data = out.fileHandleForReading.readDataToEndOfFile()
        let errText = String(decoding: err.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
        process.waitUntilExit()
        guard process.terminationStatus == 0 else {
            throw SeedFailed(outcome: outcome, status: process.terminationStatus, stderr: errText)
        }
        return (serverURL, try JSONDecoder().decode(SeedResult.self, from: data))
    }
}

private struct SeedResult: Decodable {
    let token: String
    let userID: String
    let ids: [String: String]?

    enum CodingKeys: String, CodingKey {
        case token
        case userID = "user_id"
        case ids
    }
}

private struct SeedFailed: Error, CustomStringConvertible {
    let outcome: String
    let status: Int32
    let stderr: String

    var description: String { "monacoctl flows seed 00 \(outcome) exited \(status): \(stderr)" }
}

private struct SeededToken: MonacoAPI.AccessTokenProvider {
    let token: String

    func accessToken() async throws -> String? { token.isEmpty ? nil : token }

    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct NoHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { $0.finish() }
    }
}

private actor LoseFirstPost: ClientTransport {
    private let next = URLSessionTransport()
    private var lost = false
    private(set) var recordedID: String?
    private(set) var posts = 0

    nonisolated func send(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let (response, responseBody) = try await next.send(
            request, body: body, baseURL: baseURL, operationID: operationID
        )
        guard operationID == "postSystemPing" else { return (response, responseBody) }
        guard await loseOnce() else { return (response, responseBody) }
        if let responseBody {
            let data = try await Data(collecting: responseBody, upTo: 1 << 20)
            await record(try JSONDecoder().decode(Components.Schemas.Ping.self, from: data).id)
        }
        throw URLError(.networkConnectionLost)
    }

    private func loseOnce() -> Bool {
        posts += 1
        defer { lost = true }
        return !lost
    }

    private func record(_ id: String) {
        recordedID = id
    }
}
