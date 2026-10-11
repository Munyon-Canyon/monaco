import CoreImage
import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct DepositNetworksTests {
    private let solana = DepositNetwork(id: "solana", address: "5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP")
    private let base = DepositNetwork(id: "base", address: "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")
    private let arbitrum = DepositNetwork(id: "arbitrum", address: "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")
    private let ethereum = DepositNetwork(id: "ethereum", address: "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")

    @Test func theOnrampComesBeforeTheCryptoRow() {
        #expect(DepositChooser.order == [.onramp, .crypto])
    }

    @Test func oneNetworkOpensItsAddressDirectly() {
        #expect(DepositChooser.cryptoRoute([solana]) == AnyAppRoute(DepositAddressRoute(network: solana)))
        #expect(DepositChooser.cryptoHint([solana]) == "Send USDC on Solana")
    }

    @Test func moreThanOneNetworkOpensTheListSolanaFirst() {
        let networks = [solana, base, arbitrum, ethereum]
        #expect(DepositChooser.cryptoRoute(networks) == AnyAppRoute(DepositNetworksRoute(networks: networks)))
        #expect(DepositChooser.cryptoHint(networks) == "Solana, Base, Arbitrum or Ethereum")
    }

    @Test func eachNetworkShowsItsAddressWithCopyAndQR() throws {
        for network in [solana, base] {
            let content = DepositAddressCard.Content.resolve(address: network.address, gaveUp: false)
            #expect(content == .ready(network.address))
            let image = try #require(DepositQRCode.image(for: network.address))
            #expect(Self.decode(image) == [network.address])
        }
    }

    @Test func theBaseScreenNamesBaseAndSaysItArrivesInMinutes() {
        #expect(DepositContent.title(on: base.displayName) == "Send USDC on Base")
        #expect(DepositAddressCard.networkNote(on: base.displayName) == "Only send USDC on Base to this address.")
        #expect(DepositAddressCard.arrivalNote(for: base) == "Arrives in a few minutes")
        #expect(DepositContent.steps(on: base.displayName).first == "Send USDC on Base")
    }

    @Test func solanaHasNoArrivalNote() {
        #expect(DepositAddressCard.arrivalNote(for: solana) == nil)
        #expect(DepositAddressCard.networkNote == "Only send USDC on Solana to this address.")
    }

    @Test func theBaseScreenSaysTheFeeComesOutOfTheDeposit() {
        let paid = DepositNetwork(id: "base", address: base.address, estimatedFeeMicros: 163_098)
        #expect(DepositAddressCard.feeNote(for: paid) == "Fee about $0.16. It comes out of what you send.")
    }

    @Test func noFeeNoteForSolanaNoEstimateOrUnderACent() {
        let tiny = DepositNetwork(id: "base", address: base.address, estimatedFeeMicros: 4_999)
        #expect(DepositAddressCard.feeNote(for: solana) == nil)
        #expect(DepositAddressCard.feeNote(for: base) == nil)
        #expect(DepositAddressCard.feeNote(for: tiny) == nil)
    }

    @Test func theNewCopyPassesTheMainFlowAudit() throws {
        let networks = [solana, base, arbitrum, ethereum]
        let paid = DepositNetwork(id: "base", address: base.address, estimatedFeeMicros: 163_098)
        var strings = [
            DepositChooser.cryptoTitle, DepositChooser.cryptoHint(networks), DepositNetworksView.title,
            DepositAddressCard.evmArrival, PlatformBalanceCard.arrivingLabel,
            try #require(DepositAddressCard.feeNote(for: paid)),
        ]
        for network in networks {
            strings += [
                DepositContent.title(on: network.displayName),
                DepositAddressCard.networkNote(on: network.displayName),
            ]
            strings += DepositContent.steps(on: network.displayName)
        }
        #expect(MainFlowCopyAudit.stringsAreClean(strings))
        for word in ["bridge", "token"] {
            #expect(!strings.contains { $0.lowercased().contains(word) })
        }
    }

    private static func decode(_ image: CIImage) -> [String] {
        let scaled = image.transformed(by: CGAffineTransform(scaleX: 8, y: 8))
        let padded = scaled.composited(
            over: CIImage(color: .white).cropped(to: scaled.extent.insetBy(dx: -32, dy: -32)))
        let detector = CIDetector(
            ofType: CIDetectorTypeQRCode, context: nil, options: [CIDetectorAccuracy: CIDetectorAccuracyHigh])
        return detector?.features(in: padded).compactMap { ($0 as? CIQRCodeFeature)?.messageString } ?? []
    }
}

struct DepositArrivingTests {
    private func balance(arriving: Int64) -> AccountBalance {
        AccountBalance(
            availableMicros: 10_000_000, onChainMicros: 10_000_000, inFlightMicros: 0, arrivingMicros: arriving,
            depositAddress: "x", asOf: Date())
    }

    @Test func theArrivingRowShowsWhenSomethingIsOnItsWay() {
        let card = PlatformBalanceCard(state: .loaded(balance(arriving: 4_500_000)))
        #expect(card.arrivingMicros == 4_500_000)
        #expect(PlatformBalanceCard.showsArriving(display: card.display, micros: card.arrivingMicros))
    }

    @Test func theArrivingRowIsGoneWhenNothingIsArriving() {
        let card = PlatformBalanceCard(state: .loaded(balance(arriving: 0)))
        #expect(!PlatformBalanceCard.showsArriving(display: card.display, micros: card.arrivingMicros))
    }

    @Test func aBalanceThatDidNotLoadShowsNoArrivingRow() {
        #expect(!PlatformBalanceCard.showsArriving(display: .loading, micros: 4_500_000))
        #expect(!PlatformBalanceCard.showsArriving(display: .unavailable, micros: 4_500_000))
    }
}
