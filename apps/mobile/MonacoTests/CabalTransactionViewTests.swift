import Testing

@testable import Monaco

@MainActor
struct CabalTransactionViewTests {
    @Test func theTransactionRouteOpensTheCabalReceipt() {
        let view: Any = TransactionRoute(cabalID: "cabal-1", transactionID: "activity-1").destination()
        #expect(view is CabalTransactionView)
    }
}
