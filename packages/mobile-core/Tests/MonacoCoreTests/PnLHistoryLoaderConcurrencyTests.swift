import Foundation
import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import OpenAPIRuntime
import XCTest

@testable import MonacoCore

final class PnLHistoryLoaderConcurrencyTests: XCTestCase {
    private typealias PotHistory = Components.Schemas.CabalValueHistory

    private static let cabalID = Components.Schemas.CabalRef.sampleAlpha.id

    func testNoMoreThanFourValueHistoryRequestsAreInFlight() async throws {
        let transport = InFlightTransport(body: try encode(PotHistory.sample(range: ._1m)))
        let loader = PnLHistoryLoader(api: api(transport), hints: FakeHintStream())
        let subjects = (0..<10).map { PnLHistoryLoader.Subject.cabal(id: "cabal-\($0)") }

        let curves = try await loader.show(subjects, range: .oneMonth)

        XCTAssertEqual(curves?.count, 10)
        let peak = await transport.peak
        XCTAssertEqual(peak, 4)
    }

    func testOneFailedCabalLeavesTheOthersCurves() async throws {
        let transport = StubTransport(scripted: [try pot(._1m), .json(.internalServerError, "{}"), try pot(._1m)])
        let loader = PnLHistoryLoader(api: api(transport), hints: FakeHintStream())
        let subjects = (0..<3).map { PnLHistoryLoader.Subject.cabal(id: "cabal-\($0)") }

        let curves = try await loader.show(subjects, range: .oneMonth)

        XCTAssertEqual(curves?.count, 2)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths.count, 3)
        let dropped = try XCTUnwrap(subjects.first { curves?[$0] == nil })
        let cached = await loader.cached(dropped, range: .oneMonth)
        XCTAssertNil(cached)
    }

    func testARateLimitedCabalIsRetriedOnceAfterRetryAfter() async throws {
        let sleeps = Sleeps()
        let transport = StubTransport(scripted: [try rateLimited(retryAfter: "3"), try pot(._1m)])
        let loader = PnLHistoryLoader(api: api(transport), hints: FakeHintStream(), sleep: { await sleeps.add($0) })

        let curve = try await loader.show(.cabal(id: Self.cabalID), range: .oneMonth)

        XCTAssertEqual(curve, ValueCurve(PotHistory.sample()))
        let waited = await sleeps.all
        XCTAssertEqual(waited, [.seconds(3)])
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testASecondRateLimitDropsOnlyThatCabal() async throws {
        let sleeps = Sleeps()
        let transport = StubTransport(scripted: [
            try pot(._1m), try rateLimited(retryAfter: "60"), try rateLimited(retryAfter: "60"),
        ])
        let loader = PnLHistoryLoader(api: api(transport), hints: FakeHintStream(), sleep: { await sleeps.add($0) })
        let subjects = (0..<2).map { PnLHistoryLoader.Subject.cabal(id: "cabal-\($0)") }

        let curves = try await loader.show(subjects, range: .oneMonth)

        XCTAssertEqual(curves?.count, 1)
        let waited = await sleeps.all
        XCTAssertEqual(waited, [.seconds(5)])
        let count = await transport.sent.count
        XCTAssertEqual(count, 3)
    }

    func testEveryCabalFailingThrows() async throws {
        let transport = StubTransport(.json(.internalServerError, "{}"))
        let loader = PnLHistoryLoader(api: api(transport), hints: FakeHintStream())

        do {
            _ = try await loader.show([.cabal(id: "a"), .cabal(id: "b")], range: .oneMonth)
            XCTFail("want a failure")
        } catch is APIError {}
    }

    private actor Sleeps {
        private(set) var all: [Duration] = []

        func add(_ duration: Duration) { all.append(duration) }
    }

    private actor InFlightTransport: ClientTransport {
        private let body: String
        private var current = 0
        private(set) var peak = 0

        init(body: String) { self.body = body }

        func send(_ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String) async throws
            -> (HTTPResponse, HTTPBody?)
        {
            current += 1
            peak = max(peak, current)
            for _ in 0..<50 { await Task.yield() }
            current -= 1
            var response = HTTPResponse(status: .ok)
            response.headerFields[.contentType] = "application/json"
            return (response, HTTPBody(body))
        }
    }

    private func rateLimited(retryAfter: String) throws -> StubTransport.Reply {
        let problem = Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Too many requests", status: 429, code: .rateLimited,
            message: "Slow down.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: true)
        var response = HTTPResponse(status: .tooManyRequests)
        response.headerFields[.contentType] = "application/problem+json"
        response.headerFields[.retryAfter] = retryAfter
        return .response(response, try JSONEncoder().encode(problem))
    }

    private func api(_ transport: any ClientTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func pot(_ range: PotHistory.RangePayload) throws -> StubTransport.Reply {
        .json(.ok, try encode(PotHistory.sample(range: range)))
    }

    private func encode(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }
}
