import Foundation
import HTTPTypes
import MonacoTestSupport
import OpenAPIRuntime
import XCTest

@testable import MonacoAPI

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
            case .accessRequestNotPending, .accountBanned, .accountDeleted, .accountHasBalance, .accountHasPositions,
                .accountStatusTransition, .accountSuspended, .alreadyMember, .analyticsPii, .apnsAuthFailed,
                .apnsUnavailable, .assetNotFound, .assetUntradable, .authStateTransition, .cabalBanned,
                .cabalNotFound,
                .calendarExpired, .cannotFollowSelf, .cannotRevokeAccess, .clientClosed, .conservationBroken,
                .dbSchemaBehind,
                .dbUnavailable, .decodeFailed, .displayNameInvalid, .forbidden, .handleInvalid,
                .handleRequired, .handleReserved, .handleTaken, .handleTooSoon, .idempotencyInFlight,
                .idempotencyMismatch, ._internal, .invalidAddress, .invalidConfig, .invalidInput,
                .inviteExpired, .joinNeedsRequest, .jupiterRejected, .jupiterUnavailable, .leaveCreatorWithMembers,
                .leaveHoldsShares, .leaveLastMemberPotNotEmpty, .ledgerUnbalanced, .liveSwapExists,
                .loginMethodNotAllowed,
                .noRoute, .notAVoter, .notCabalCreator, .notCabalMember, .notFound,
                .notProposer, .panic, .phoneNotLinked, .photoInvalid, .postHogRejected,
                .postHogUnavailable, .potExceeded, .potValueZero, .privyUnavailable, .proposalClosed,
                .proposalNotFound, .rateLimited, .referralCodePending, .referralCodeUnknown, .relayerUnderfunded,
                .requestNotNeeded, .requestPending,
                .rpcUnavailable, .sessionRequired, .slippageExceeded, .swapFailed, .swapNotFound,
                .swapNotRetryable, .unauthorized, .upstreamTimeout, .upstreamUnavailable, .userBanned,
                .userNotFound,
                .versionConflict, .walletMismatch, .withdrawNotAllowed, .xNotLinked:
                break
            }
        }
    }

    func testAProblemResponseThrowsTheGeneratedProblemAsProblemError() async throws {
        let client = APIClient(
            serverURL: testServerURL,
            tokens: StubTokenProvider(token: nil),
            transport: try StubTransport.problem(upstreamTimeout)
        ).client

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
        let client = APIClient(
            serverURL: testServerURL,
            tokens: StubTokenProvider(token: nil),
            transport: StubTransport(
                status: .unprocessableContent,
                contentType: "application/problem+json",
                body: Data(body.utf8)
            )
        ).client

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
            let client = APIClient(
                serverURL: testServerURL,
                tokens: StubTokenProvider(token: nil),
                transport: StubTransport(status: .badGateway, contentType: contentType, body: Data("<html>".utf8))
            ).client

            do {
                _ = try await client.getHealthz()
                XCTFail("expected the call to throw for Content-Type \"\(contentType)\"")
            } catch {
                XCTAssertNil(ProblemError(error), "Content-Type \"\(contentType)\"")
            }
        }
    }

    func testASuccessPassesThrough() async throws {
        let client = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: nil), transport: StubTransport.ok("ok\n")
        ).client

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
