import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import Synchronization
import XCTest

final class FirstSignInRaceTests: XCTestCase {
    func testAStreamSessionRequired401MidSessionCreateLeavesTheSessionToComplete() async throws {
        let server = FirstSignInServer()
        let session = SessionTask()
        let tokens = SigningOutTokens(onEnd: {
            session.cancel()
            server.noteSignedOut()
        })
        let api = APIClient(serverURL: testServerURL, tokens: tokens, transport: server)
        let stream = HintStream(
            serverURL: testServerURL,
            transport: server,
            token: { try await tokens.accessToken() },
            refresh: { try await tokens.refreshedToken(replacing: $0) },
            endSession: { rejected in
                if let rejected { await tokens.endSession(rejectedToken: rejected) }
            },
            random: { 0.001 }
        )

        await stream.start()
        let opened = session.start { try await SessionAPI(api: api).openSession() }
        let profile = try await opened.value
        let connected = await eventually { server.streamOpened }
        await stream.stop()

        XCTAssertEqual(profile.handle, "newcomer")
        XCTAssertGreaterThanOrEqual(server.streamRejections, 2, "the stream was refused while the session was open")
        XCTAssertFalse(server.signedOut)
        XCTAssertTrue(connected, "the stream connects once the session exists")
    }

    private func eventually(_ condition: () async -> Bool) async -> Bool {
        let deadline = ContinuousClock.now + .seconds(5)
        while ContinuousClock.now < deadline {
            if await condition() { return true }
            await Task.yield()
        }
        return false
    }
}

private final class SigningOutTokens: MonacoAPI.AccessTokenProvider {
    private let onEnd: @Sendable () -> Void

    init(onEnd: @escaping @Sendable () -> Void) {
        self.onEnd = onEnd
    }

    func accessToken() async throws -> String? { "token-1" }
    func refreshedToken(replacing _: String) async throws -> String? { "token-2" }
    func endSession(rejectedToken _: String) async { onEnd() }
    func endSession() async { onEnd() }
}

private final class SessionTask: Sendable {
    private let task = Mutex<Task<SessionProfile, any Error>?>(nil)

    func start(_ work: @escaping @Sendable () async throws -> SessionProfile) -> Task<SessionProfile, any Error> {
        let started = Task { try await work() }
        task.withLock { $0 = started }
        return started
    }

    func cancel() {
        task.withLock { $0 }?.cancel()
    }
}

private final class FirstSignInServer: ClientTransport {
    private struct State {
        var hasRow = false
        var streamRejections = 0
        var streamRequests = 0
        var streamOpened = false
        var signedOut = false
    }

    private let state = Mutex(State())
    private let signals: AsyncStream<Void>
    private let signal: AsyncStream<Void>.Continuation

    init() {
        (signals, signal) = AsyncStream.makeStream(of: Void.self)
    }

    var streamRejections: Int { state.withLock { $0.streamRejections } }
    var streamOpened: Bool { state.withLock { $0.streamOpened } }
    var signedOut: Bool { state.withLock { $0.signedOut } }

    func noteSignedOut() {
        state.withLock { $0.signedOut = true }
        signal.yield()
    }

    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        switch request.path {
        case "/v1/stream": return try stream()
        case "/v1/auth/session": return try await createSession()
        default: throw URLError(.badURL)
        }
    }

    private func stream() throws -> (HTTPResponse, HTTPBody?) {
        let refused = state.withLock { state -> Bool in
            state.streamRequests += 1
            if state.hasRow { state.streamOpened = true } else { state.streamRejections += 1 }
            return !state.hasRow
        }
        signal.yield()
        if refused { return try problem() }
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "text/event-stream"
        let (bytes, _) = AsyncStream.makeStream(of: ArraySlice<UInt8>.self)
        return (response, HTTPBody(bytes, length: .unknown, iterationBehavior: .single))
    }

    private func createSession() async throws -> (HTTPResponse, HTTPBody?) {
        for await _ in signals where state.withLock({ $0.streamRequests >= 3 || $0.signedOut }) { break }
        try Task.checkCancellation()
        state.withLock { $0.hasRow = true }
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(Data(Self.meJSON.utf8)))
    }

    private func problem() throws -> (HTTPResponse, HTTPBody?) {
        let problem = Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Unauthorized", status: 401, code: .sessionRequired,
            message: "Sign in to continue.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: false
        )
        var response = HTTPResponse(status: .unauthorized)
        response.headerFields[.contentType] = "application/problem+json"
        return (response, HTTPBody(try JSONEncoder().encode(problem)))
    }

    private static let meJSON = """
        {"id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"newcomer","display_name":"New Comer",\
        "auth_state":"CREATED","account_status":"active","member_wallet_address":"wallet-1",\
        "phone_linked":false,"created_at":"2026-09-30T12:00:00Z"}
        """
}
