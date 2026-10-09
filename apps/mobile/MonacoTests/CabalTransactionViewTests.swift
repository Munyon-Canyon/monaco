import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct CabalTransactionViewTests {
    @Test func theTransactionRouteOpensTheCabalReceipt() {
        let view: Any = TransactionRoute(cabalID: "cabal-1", transactionID: "activity-1").destination()
        #expect(view is CabalTransactionView)
    }

    @Test(.timeLimit(.minutes(1)))
    func aFundRowPendingAtLookupIsDoneOnTheReceiptAfterAHintRefresh() async throws {
        typealias Activity = Components.Schemas.CabalActivity
        func page(_ status: Activity.StatusPayload) throws -> StubTransport.Reply {
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            let fund = Activity.sample(3, kind: .fund, status: status, asset: nil, micros: 5_000_000, actor: nil)
            let body = try encoder.encode(Components.Schemas.CabalActivityPage(items: [fund], nextCursor: nil))
            return .json(.ok, String(decoding: body, as: UTF8.self))
        }
        let transport = StubTransport(scripted: [try page(.pending), try page(.confirmed)])
        let hints = EmittingHintSource()
        let model = CabalActivityModel(
            cabalID: "cabal-1",
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints, clock: { Date(timeIntervalSince1970: 1_791_100_800) })

        let id = Activity.sample(3, kind: .fund, status: .pending, asset: nil, micros: nil, actor: nil).id
        guard case .found(let opened) = await model.lookup(id: id) else {
            Issue.record("the fund row should be found")
            return
        }
        #expect(opened.status.receiptLabel == "Pending")

        let observer = Task { await model.observe() }
        defer { observer.cancel() }
        while hints.subscribers < 2 { await Task.yield() }
        hints.send(.changed(.cabal("cabal-1"), what: "activity_changed", id: "1"))
        while model.rows.first?.status != .confirmed { await Task.yield() }

        let live = model.rows.first { $0.id == opened.id } ?? opened
        #expect(live.status.receiptLabel == "Done")
    }
}
