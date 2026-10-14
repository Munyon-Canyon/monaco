import Foundation
import XCTest

@testable import MonacoCore

final class WithdrawNetworkTests: XCTestCase {
    private let solanaAddress = "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"

    func testKeccakMatchesTheKnownDigestOfTheEmptyString() {
        let digest = Keccak256.hash([]).map { String(format: "%02x", $0) }.joined()
        XCTAssertEqual(digest, "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470")
    }

    func testKeccakPadsAMessageOneByteShortOfABlock() {
        let digest = Keccak256.hash([UInt8](repeating: 0x61, count: 135)).map { String(format: "%02x", $0) }.joined()
        XCTAssertEqual(digest, "34367dc248bbd832f4e3e69dfaac2f92638bd0bbd18f2912ba4ef454919cf446")
    }

    func testEIP55VectorsChecksumAndValidate() {
        let vectors = [
            "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
            "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
            "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
            "0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
        ]
        for vector in vectors {
            XCTAssertEqual(EVMAddress.checksummed(String(vector.dropFirst(2))), vector)
            XCTAssertEqual(EVMAddress.validate(vector), .success(vector))
        }
    }

    func testAnEVMAddressIsAcceptedInOneCaseAndRefusedWithABrokenChecksum() {
        XCTAssertEqual(
            EVMAddress.validate(" 0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed\n"),
            .success("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"))
        XCTAssertEqual(
            EVMAddress.validate("0x5AAEB6053F3E94C9B9A09F33669435E7EF1BEAED"),
            .success("0x5AAEB6053F3E94C9B9A09F33669435E7EF1BEAED"))
        XCTAssertEqual(EVMAddress.validate("0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD"), .failure(.badChecksum))
        XCTAssertEqual(EVMAddress.validate("5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"), .failure(.notAnAddress))
        XCTAssertEqual(EVMAddress.validate("0x5aaeb6053f3e94c9b9a09f33669435e7ef1bea"), .failure(.notAnAddress))
        XCTAssertEqual(EVMAddress.validate("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beazz"), .failure(.notAnAddress))
        XCTAssertEqual(EVMAddress.validate("  "), .failure(.empty))
        XCTAssertEqual(
            EVMAddress.validate("0x5aaeb6053f3e94c9b9a09f33669435e7ef1bea\u{FF45}\u{FF44}"), .failure(.notAnAddress))
    }

    func testEachNetworkValidatesItsOwnKindOfAddress() {
        let evm = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"
        XCTAssertEqual(WithdrawNetwork.solana.validate(solanaAddress, ownDepositAddress: nil), .success(solanaAddress))
        XCTAssertEqual(
            WithdrawNetwork.solana.validate(evm, ownDepositAddress: nil), .failure(.solana(.badCharacter("0"))))
        for network in [WithdrawNetwork.base, .arbitrum, .ethereum] {
            XCTAssertEqual(network.validate(evm, ownDepositAddress: nil), .success(evm))
            XCTAssertEqual(network.validate(solanaAddress, ownDepositAddress: nil), .failure(.evm(.notAnAddress)))
        }
        XCTAssertEqual(
            WithdrawNetwork.solana.validate(solanaAddress, ownDepositAddress: solanaAddress),
            .failure(.solana(.ownDepositAddress)))
    }

    func testCopyNamesTheNetworkAndMarksAnEVMEstimateAsApproximate() {
        XCTAssertEqual(WithdrawNetwork.allCases.map(\.name), ["Solana", "Base", "Arbitrum", "Ethereum", "Monad"])
        XCTAssertEqual(WithdrawNetwork.base.placeholder, "USDC address on Base")
        XCTAssertEqual(WithdrawNetwork.base.caveat, "A Base address that accepts USDC. Transfers can't be undone.")
        XCTAssertEqual(
            WithdrawNetwork.ethereum.caveat, "An Ethereum address that accepts USDC. Transfers can't be undone.")
        XCTAssertEqual(
            WithdrawNetwork.arbitrum.message(for: .evm(.notAnAddress)),
            "Check the address. It doesn't look like an Arbitrum address.")
        XCTAssertEqual(
            WithdrawNetwork.base.message(for: .evm(.notAnAddress)),
            "Check the address. It doesn't look like a Base address.")
        XCTAssertEqual(WithdrawNetwork.arbitrum.receiveLine(receiveMicros: 9_992_500), "You'll receive ~$9.99")
        XCTAssertEqual(WithdrawNetwork.solana.receiveLine(receiveMicros: 10_000_000), "You'll receive $10.00")
        XCTAssertTrue(WithdrawAddressProblem.evm(.empty).isEmpty)
        XCTAssertFalse(WithdrawAddressProblem.evm(.badChecksum).isEmpty)
    }

    func testAnEVMRequestNamesItsNetworkAndASolanaRequestStaysAsBefore() {
        let evm = WithdrawNetwork.ethereum.request(micros: 2_000_000, toAddress: "0xabc")
        XCTAssertEqual(evm.destinationKind, .evm)
        XCTAssertEqual(evm.network, .ethereum)
        XCTAssertEqual(evm.amountMicros, "2000000")
        let solana = WithdrawNetwork.solana.request(micros: 2_000_000, toAddress: solanaAddress)
        XCTAssertNil(solana.destinationKind)
        XCTAssertNil(solana.network)
    }
}
