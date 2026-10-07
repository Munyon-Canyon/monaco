import Foundation
import MonacoAPI
import Testing

@testable import Monaco

struct DepositAddressTests {
    @Test func aRealAddressComesBackTrimmed() {
        #expect(DepositAddress.usable("  7Yk3Qn5wF2c  ") == "7Yk3Qn5wF2c")
    }

    /// A wallet still being made comes back empty, and dev environments hand out "FAKE…".
    /// Neither may reach the clipboard, because real money gets sent to what is on this screen.
    @Test func aPlaceholderIsNotAnAddress() {
        #expect(DepositAddress.usable(nil) == nil)
        #expect(DepositAddress.usable("") == nil)
        #expect(DepositAddress.usable("   ") == nil)
        #expect(DepositAddress.usable("FAKE_WALLET_123") == nil)
        #expect(DepositAddress.usable("  FAKE123  ") == nil)
    }
}

struct DepositAddressCardTests {
    private let wallet = "7Yk3Qn5wF2cXe9Lp4RtUv8Hs6JdBm1ZaNq3GfKyWo2Tc"

    @Test func theAddressComesFromTheProfileNotTheBalance() {
        #expect(DepositAddressCard.Content.resolve(address: wallet) == .ready(wallet))
    }

    @Test func anEmptyProfileAddressIsUnavailable() {
        let unavailable = DepositAddressCard.Content.unavailable("Couldn't load your deposit address.")
        #expect(DepositAddressCard.Content.resolve(address: "") == unavailable)
        #expect(DepositAddressCard.Content.resolve(address: nil) == unavailable)
        #expect(DepositAddressCard.Content.resolve(address: "FAKE_WALLET_123") == unavailable)
    }

    @Test func aFailedBalanceDoesNotHideTheAddress() {
        let content = DepositContent(
            address: wallet, state: .failed(.transport(URLError(.cannotConnectToHost))),
            onCopy: { _ in }, onRetryAddress: {}, onRetryBalance: {})
        #expect(content.address == wallet)
        #expect(DepositAddressCard.Content.resolve(address: content.address) == .ready(wallet))
        #expect(PlatformBalanceCard(state: content.state).display == .unavailable)
    }
}
