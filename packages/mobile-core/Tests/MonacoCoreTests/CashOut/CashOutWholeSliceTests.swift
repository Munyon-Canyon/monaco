import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CashOutWholeSliceTests: XCTestCase {
    private let cabalID = "01890a5d-ac96-774b-bcce-b302099a8059"

    private func model(_ preview: Components.Schemas.CashOutPreview, payout: String) async throws
        -> (CashOutModel, StubTransport)
    {
        let transport = StubTransport(scripted: [
            try CashOutModelTests.json(.ok, preview),
            try CashOutModelTests.json(
                .accepted, Components.Schemas.CashOutJob.sample(status: .started, payoutMicros: payout)),
        ])
        let model = CashOutModel(
            cabalID: cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: FakeHintStream()
        )
        await model.load()
        return (model, transport)
    }

    private func lastBody(_ transport: StubTransport) async throws -> NSDictionary {
        let bodies = await transport.sentBodies
        let data = try XCTUnwrap(bodies.last ?? nil)
        return try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? NSDictionary)
    }

    func testAWholeSliceUnderTheMinimumSendsAllTrueAndAPartialSendsNothing() async throws {
        let small = Components.Schemas.CashOutPreview(
            sliceMicros: "40000", shareUnits: "40000", minMicros: "100000", pause: nil)
        let (model, transport) = try await model(small, payout: "40000")

        XCTAssertFalse(model.maySubmit(enteredMicros: 20_000))
        let partial = await model.submit(enteredMicros: 20_000)
        XCTAssertNil(partial)
        XCTAssertTrue(model.maySubmit(enteredMicros: 40_000))
        _ = await model.submit(enteredMicros: 40_000)

        let body = try await lastBody(transport)
        XCTAssertEqual(body, ["all": true])
    }

    func testAZeroValueSliceStillSendsAllTrue() async throws {
        let zero = Components.Schemas.CashOutPreview(sliceMicros: "0", shareUnits: "7", minMicros: "100000", pause: nil)
        let (model, transport) = try await model(zero, payout: "0")

        XCTAssertTrue(model.maySubmit(enteredMicros: model.preview?.entered(typedMicros: 0) ?? -1))
        _ = await model.submit(enteredMicros: 0)

        let body = try await lastBody(transport)
        XCTAssertEqual(body, ["all": true])
    }
}
