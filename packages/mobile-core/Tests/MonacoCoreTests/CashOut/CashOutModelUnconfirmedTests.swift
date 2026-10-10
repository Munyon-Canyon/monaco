import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

extension CashOutModelTests {
    func testAChangedAmountAfterAnUnconfirmedCashOutSendsNothing() async throws {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            .failure(URLError(.networkConnectionLost)),
        ])
        let model = makeModel(transport)
        await model.load()

        let first = await model.submit(enteredMicros: 1_000_000)
        let second = await model.submit(enteredMicros: 2_000_000)

        XCTAssertEqual(first, .refused(MoneyFlowCopy.unconfirmed.summary))
        XCTAssertEqual(second, .refused(MoneyFlowCopy.unconfirmed.summary))
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testTheSameAmountAfterAnUnconfirmedCashOutResendsTheFirstRequest() async throws {
        let smaller = Components.Schemas.CashOutPreview(
            sliceMicros: "150000000", shareUnits: "150000000", minMicros: "100000", pause: nil)
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            .failure(URLError(.networkConnectionLost)),
            try Self.json(.ok, smaller),
            try Self.json(.accepted, Components.Schemas.CashOutJob.sample(status: .started)),
        ])
        let model = makeModel(transport)
        await model.load()
        _ = await model.submit(enteredMicros: 200_100_000)

        await model.load()

        XCTAssertTrue(model.maySubmit(enteredMicros: 200_100_000))
        let result = await model.submit(enteredMicros: 200_100_000)
        guard case .started = result else { return XCTFail("expected a job, got \(String(describing: result))") }
        let posts = try await posts(transport)
        XCTAssertEqual(posts.map(\.body), [["all": true], ["all": true]])
        XCTAssertNotNil(posts[0].key)
        XCTAssertEqual(posts[0].key, posts[1].key)
    }

    func testAFinalAnswerLetsAChangedAmountGoOut() async throws {
        let transport = StubTransport(scripted: [
            try Self.json(.ok, Components.Schemas.CashOutPreview.sample),
            .failure(URLError(.networkConnectionLost)),
            try problem(.cashOutInProgress, status: 409, "busy"),
            try Self.json(.accepted, Components.Schemas.CashOutJob.sample(status: .started)),
        ])
        let model = makeModel(transport)
        await model.load()
        _ = await model.submit(enteredMicros: 1_000_000)

        let second = await model.submit(enteredMicros: 1_000_000)
        let third = await model.submit(enteredMicros: 2_000_000)

        XCTAssertEqual(second, .refused("A cash out is already running for this cabal."))
        guard case .started = third else { return XCTFail("expected a job, got \(String(describing: third))") }
        let posts = try await posts(transport)
        let amounts = ["1000000", "1000000", "2000000"].map { ["usdc_micros": $0] as NSDictionary }
        XCTAssertEqual(posts.map(\.body), amounts)
        XCTAssertEqual(posts[0].key, posts[1].key)
        XCTAssertNotEqual(posts[1].key, posts[2].key)
    }

    func posts(_ transport: StubTransport) async throws -> [(key: String?, body: NSDictionary)] {
        let name = try idempotencyKey()
        let sent = await transport.sent
        let bodies = await transport.sentBodies
        return try zip(sent, bodies).filter { $0.0.method == .post }.map { request, body in
            let json = try JSONSerialization.jsonObject(with: try XCTUnwrap(body))
            return (request.headerFields[name], try XCTUnwrap(json as? NSDictionary))
        }
    }
}
