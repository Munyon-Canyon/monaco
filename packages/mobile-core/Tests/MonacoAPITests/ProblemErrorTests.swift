import Foundation
import HTTPTypes
import MonacoAPI
import OpenAPIRuntime
import XCTest

final class ProblemErrorTests: XCTestCase {
    typealias ErrorCode = Components.Schemas.ErrorCode

    private let upstreamTimeout = Components.Schemas.Problem(
        _type: .about_colon_blank,
        title: "Service Unavailable",
        status: 503,
        code: .upstreamTimeout,
        message: "A provider timed out. Try again shortly.",
        traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
        retryable: true
    )

    func testANewErrorCodeFailsThisBuildUntilListed() {
        for code in ErrorCode.allCases {
            switch code {
            case .clientClosed,
                 .dbSchemaBehind,
                 .dbUnavailable,
                 .decodeFailed,
                 .forbidden,
                 .idempotencyInFlight,
                 .idempotencyMismatch,
                 ._internal,
                 .invalidInput,
                 .jupiterRejected,
                 .jupiterUnavailable,
                 .notFound,
                 .panic,
                 .unauthorized,
                 .upstreamTimeout,
                 .upstreamUnavailable,
                 .versionConflict:
                break
            }
        }
    }

    func testAProblemResponseThrowsTheGeneratedProblemAsProblemError() async throws {
        let client = Client.monaco(
            serverURL: testServerURL,
            accessToken: { nil },
            transport: try StubTransport.problem(upstreamTimeout)
        )

        let problem = await problemThrown { _ = try await client.getHealthz() }

        XCTAssertEqual(problem, ProblemError(upstreamTimeout))
        XCTAssertEqual(problem?.code, ProblemError.Code.known(.upstreamTimeout))
        XCTAssertEqual(problem?.localizedDescription, "A provider timed out. Try again shortly.")
    }

    func testAnUnknownCodeDecodesToTheUnrecognizedFallback() async throws {
        let body = """
        {"type":"about:blank","title":"Unprocessable Content","status":422,\
        "code":"code_from_a_newer_server","message":"Voting has closed.",\
        "trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","retryable":false}
        """
        let client = Client.monaco(
            serverURL: testServerURL,
            accessToken: { nil },
            transport: StubTransport(
                status: .unprocessableContent,
                contentType: "application/problem+json",
                body: Data(body.utf8)
            )
        )

        let problem = await problemThrown { _ = try await client.getHealthz() }

        XCTAssertEqual(
            problem,
            ProblemError(
                status: 422,
                code: .unrecognized("code_from_a_newer_server"),
                message: "Voting has closed.",
                traceID: "4bf92f3577b34da6a3ce929d0e0e4736",
                retryable: false
            )
        )
    }

    func testAnErrorWithABlankContentTypeIsNotAProblem() async throws {
        for contentType in ["", ";"] {
            let client = Client.monaco(
                serverURL: testServerURL,
                accessToken: { nil },
                transport: StubTransport(status: .badGateway, contentType: contentType, body: Data("<html>".utf8))
            )

            do {
                _ = try await client.getHealthz()
                XCTFail("expected the call to throw for Content-Type \"\(contentType)\"")
            } catch {
                XCTAssertNil(ProblemError(error), "Content-Type \"\(contentType)\"")
            }
        }
    }

    func testASuccessPassesThrough() async throws {
        let client = Client.monaco(serverURL: testServerURL, accessToken: { nil }, transport: StubTransport.ok("ok\n"))

        let body = try await client.getHealthz().ok.body.plainText

        let text = try await String(collecting: body, upTo: 16)
        XCTAssertEqual(text, "ok\n")
    }

    private func problemThrown(_ call: () async throws -> Void) async -> ProblemError? {
        do {
            try await call()
        } catch {
            return ProblemError(error)
        }
        XCTFail("expected the call to throw")
        return nil
    }
}
