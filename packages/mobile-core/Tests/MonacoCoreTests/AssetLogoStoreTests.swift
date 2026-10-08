import Foundation
import HTTPTypes
import MonacoAPI
import MonacoTestSupport
import OpenAPIRuntime
import XCTest

@testable import MonacoCore

final class AssetLogoStoreTests: XCTestCase {
    func testAResolvedSymbolIsNeverLookedUpAgain() async throws {
        let transport = CountingTransport(logos: ["GOOGLx": "https://cdn.example.com/GOOGLx.png"])
        let store = AssetLogoStore(api: api(transport))

        let first = await store.logos(for: ["GOOGLx"])
        let second = await store.logos(for: ["GOOGLx"])

        XCTAssertEqual(first["GOOGLx"], URL(string: "https://cdn.example.com/GOOGLx.png"))
        XCTAssertEqual(second, first)
        let calls = await transport.calls
        XCTAssertEqual(calls, 1)
    }

    func testASymbolWithNoLogoIsNotLookedUpAgain() async throws {
        let transport = CountingTransport(logos: ["tSpaceX": nil])
        let store = AssetLogoStore(api: api(transport))

        let first = await store.logos(for: ["tSpaceX"])
        let second = await store.logos(for: ["tSpaceX"])

        XCTAssertTrue(first.isEmpty)
        XCTAssertTrue(second.isEmpty)
        let calls = await transport.calls
        XCTAssertEqual(calls, 1)
    }

    func testAFailedLookupKeepsTheTileAndIsNotRetried() async throws {
        let transport = CountingTransport(failing: ["AAPLx"])
        let store = AssetLogoStore(api: api(transport))

        let first = await store.logos(for: ["AAPLx"])
        let second = await store.logos(for: ["AAPLx"])

        XCTAssertTrue(first.isEmpty)
        XCTAssertTrue(second.isEmpty)
        let calls = await transport.calls
        XCTAssertEqual(calls, 1)
    }

    func testConcurrentAskersForOneSymbolShareOneLookup() async throws {
        let transport = CountingTransport(logos: ["GOOGLx": "https://cdn.example.com/GOOGLx.png"])
        let store = AssetLogoStore(api: api(transport))

        async let a = store.logos(for: ["GOOGLx"])
        async let b = store.logos(for: ["GOOGLx", "GOOGLx"])
        _ = await (a, b)

        let calls = await transport.calls
        XCTAssertEqual(calls, 1)
    }

    func testNoMoreThanFourLookupsAreInFlight() async throws {
        let symbols = (0..<12).map { "S\($0)x" }
        let transport = CountingTransport(
            logos: Dictionary(uniqueKeysWithValues: symbols.map { ($0, "https://cdn.example.com/\($0).png") }))
        let store = AssetLogoStore(api: api(transport))

        let found = await store.logos(for: symbols)

        XCTAssertEqual(found.count, 12)
        let peak = await transport.peak
        XCTAssertLessThanOrEqual(peak, 4)
        XCTAssertGreaterThan(peak, 1)
    }

    @MainActor
    func testOnlyAModelGivenAStoreLooksLogosUpAndRefreshesDoNotRepeatIt() async throws {
        let transport = CountingTransport(logos: ["GOOGLx": "https://cdn.example.com/GOOGLx.png"])
        let bare = CabalPotModel(cabalID: "c", api: api(transport), hints: FakeHintStream())
        await bare.load()
        let bareCalls = await transport.calls
        XCTAssertEqual(bareCalls, 1)
        XCTAssertNil(bare.summary?.holdings.first?.logoURL)

        let store = AssetLogoStore(api: api(transport))
        let model = CabalPotModel(cabalID: "c", api: api(transport), hints: FakeHintStream(), logoStore: store)
        await model.load()
        let applied = await waitUntil { model.summary?.holdings.first?.logoURL != nil }
        XCTAssertTrue(applied)
        await model.load()
        await model.load()
        for _ in 0..<100 { await Task.yield() }

        let calls = await transport.calls
        XCTAssertEqual(calls, 1 + 3 + 1)
        XCTAssertEqual(model.summary?.holdings.first?.logoURL, URL(string: "https://cdn.example.com/GOOGLx.png"))
    }

    @MainActor
    private func waitUntil(_ predicate: () -> Bool) async -> Bool {
        for _ in 0..<2000 {
            if predicate() { return true }
            await Task.yield()
        }
        return false
    }

    private func api(_ transport: any ClientTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private actor CountingTransport: ClientTransport {
        private let logos: [String: String?]
        private let failing: Set<String>
        private var current = 0
        private(set) var peak = 0
        private(set) var calls = 0

        init(logos: [String: String?] = [:], failing: Set<String> = []) {
            self.logos = logos
            self.failing = failing
        }

        func send(_ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String) async throws
            -> (HTTPResponse, HTTPBody?)
        {
            calls += 1
            current += 1
            peak = max(peak, current)
            for _ in 0..<50 { await Task.yield() }
            current -= 1
            if request.path?.hasSuffix("/pot") == true {
                var response = HTTPResponse(status: .ok)
                response.headerFields[.contentType] = "application/json"
                let encoder = JSONEncoder()
                encoder.dateEncodingStrategy = .iso8601
                return (response, HTTPBody(try encoder.encode(Components.Schemas.CabalPot.sampleInvested)))
            }
            let symbol = request.path?.split(separator: "/").last.map(String.init) ?? ""
            if failing.contains(symbol) { return (HTTPResponse(status: .internalServerError), HTTPBody("{}")) }
            var detail = Components.Schemas.AssetDetail.googl
            detail.symbol = symbol
            detail.logoUrl = logos[symbol] ?? nil
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            var response = HTTPResponse(status: .ok)
            response.headerFields[.contentType] = "application/json"
            return (response, HTTPBody(try encoder.encode(detail)))
        }
    }
}
