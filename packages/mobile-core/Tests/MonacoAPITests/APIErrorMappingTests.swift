import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif
import MonacoAPI
import MonacoTestSupport
import XCTest

final class APIErrorMappingTests: XCTestCase {
    private func thrown(by reply: StubTransport.Reply) async -> APIError? {
        do {
            try await Fixtures.client(StubTransport(reply)).healthz()
        } catch {
            return error as? APIError
        }
        XCTFail("expected a throw")
        return nil
    }

    func testAKnownCodeIsAProblemWithTheKnownCode() async throws {
        guard case let .problem(problem) = await thrown(by: Fixtures.problem(404, "not_found")) else {
            return XCTFail("expected .problem")
        }
        XCTAssertEqual(problem.code, .known(.notFound))
    }

    func testAccountDeletedIsMatchedOnTheWireString() async {
        let error = await thrown(by: Fixtures.problem(403, "account_deleted"))

        XCTAssertEqual(error, .accountDeleted)
    }

    func testIdempotencyInFlightIsInFlight() async {
        let error = await thrown(by: Fixtures.problem(409, "idempotency_in_flight"))

        XCTAssertEqual(error, .inFlight)
    }

    func testAnUnknownCodeStaysAProblemWithTheUnrecognizedCode() async throws {
        guard case let .problem(problem) = await thrown(by: Fixtures.problem(422, "brand_new")) else {
            return XCTFail("expected .problem")
        }
        XCTAssertEqual(problem.code, .unrecognized("brand_new"))
        XCTAssertEqual(problem.code.wire, "brand_new")
    }

    func testATransportFailureIsTransport() async {
        let error = await thrown(by: .failure(URLError(.notConnectedToInternet)))

        XCTAssertEqual(error, .transport(URLError(.notConnectedToInternet)))
    }

    func testAnUnreadableSuccessIsDecoding() async throws {
        guard case .decoding = await thrown(by: .response(status: .ok, contentType: "application/json", body: Data("{".utf8))) else {
            return XCTFail("expected .decoding")
        }
    }

    func testTheWireCodeOfAKnownCodeIsItsRawValue() {
        XCTAssertEqual(ProblemError.Code.known(.idempotencyInFlight).wire, "idempotency_in_flight")
    }
}
