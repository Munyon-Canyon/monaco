import Foundation
import Testing

@testable import Monaco

@MainActor
struct CabalTreasurySlotTests {
    @Test func theTreasuryRowIsLive() {
        #expect(CabalTreasurySlot.isLive)
    }

    @Test func solscanLinksToTheTreasuryAccount() {
        let address = "Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf"
        #expect(
            CabalTreasurySlot.solscanURL(for: address)?.absoluteString
                == "https://solscan.io/account/Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf")
    }
}
