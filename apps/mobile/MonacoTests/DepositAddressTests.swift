import CoreImage
import Foundation
import MonacoAPI
import MonacoCore
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
        let unavailable = DepositAddressCard.Content.unavailable
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

extension DepositAddressTests {
    @Test func theQRCodeDecodesToTheExactAddress() throws {
        let address = "7Yk3Qn5wF2cH8sVbN4tRzLpXu9DmEaJgKq6TyWvB1xCe"
        let image = try #require(DepositQRCode.image(for: address))
        let scaled = image.transformed(by: CGAffineTransform(scaleX: 8, y: 8))
        let padded = scaled.composited(
            over: CIImage(color: .white).cropped(to: scaled.extent.insetBy(dx: -32, dy: -32)))
        let detector = try #require(
            CIDetector(
                ofType: CIDetectorTypeQRCode, context: nil, options: [CIDetectorAccuracy: CIDetectorAccuracyHigh])
        )
        let messages = detector.features(in: padded).compactMap { ($0 as? CIQRCodeFeature)?.messageString }
        #expect(messages == [address])
    }
}

struct DepositContentLayoutTests {
    private func balance(_ micros: Int64) -> AccountBalance {
        AccountBalance(
            availableMicros: micros, onChainMicros: micros, inFlightMicros: 0, depositAddress: "x", asOf: Date())
    }

    @Test func theBalanceComesBeforeTheAddressAndTheStepper() {
        #expect(DepositContent.order == [.balance, .address, .howItWorks])
    }

    @Test func theStepperIsThreeShortSteps() {
        #expect(DepositContent.steps == ["Send USDC on Solana", "It lands in your balance", "Fund a cabal"])
    }

    @Test func aCreditedDepositChangesTheShownBalance() {
        let before = PlatformBalanceCard(state: .loaded(balance(1_230_000))).display
        let after = PlatformBalanceCard(state: .loaded(balance(6_230_000))).display
        #expect(before == .amount(1_230_000))
        #expect(after == .amount(6_230_000))
        #expect(
            BalanceChange.detect(previous: balance(1_230_000), current: balance(6_230_000))
                == .deposited(5_000_000))
    }
}
