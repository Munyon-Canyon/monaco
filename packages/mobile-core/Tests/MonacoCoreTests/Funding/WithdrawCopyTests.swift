import Foundation
import MonacoAPI
import XCTest

@testable import MonacoCore

final class WithdrawCopyTests: XCTestCase {
    private let id = "01890a5d-ac96-774b-bcce-b302099a8057"
    private let signature = "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW"

    private func withdrawal(_ status: WithdrawalStatus, signature: String? = "sig") -> Withdrawal {
        Withdrawal(withdrawalID: id, status: status, amountMicros: 120_000_000, txSignature: signature)
    }

    func testTheEmptyLineForAnOnTopFeeAZeroFeeAndAnUnknownFee() {
        XCTAssertEqual(WithdrawCopy.emptyLine(feeMicros: 500_000, network: .solana), "Fee $0.50 · USDC on Solana")
        XCTAssertEqual(WithdrawCopy.emptyLine(feeMicros: 0, network: .solana), "No fee · USDC on Solana")
        XCTAssertEqual(
            WithdrawCopy.emptyLine(feeMicros: nil, network: .base), "Fee shown once you type · USDC on Base")
    }

    func testTheTypedLineForAnOnTopFeeAnIncludedFeeAndAZeroFee() {
        XCTAssertEqual(
            WithdrawCopy.typedLine(receiveMicros: 120_000_000, feeMicros: 500_000), "≈ 120.00 USDC · fee $0.50")
        XCTAssertEqual(
            WithdrawCopy.typedLine(receiveMicros: 9_992_500, feeMicros: 7_500), "≈ 9.99 USDC · fee $0.01")
        XCTAssertEqual(WithdrawCopy.typedLine(receiveMicros: 120_000_000, feeMicros: 0), "≈ 120.00 USDC · no fee")
        XCTAssertEqual(WithdrawCopy.typedLine(receiveMicros: 120_000_000, feeMicros: nil), "≈ 120.00 USDC")
    }

    func testTheLimitLineSlotAndButton() {
        XCTAssertEqual(
            WithdrawCopy.limitLine(maxMicros: 499_500_000, overMicros: 150_500_000),
            "You can send $499.50 · $150.50 over")
        XCTAssertEqual(WithdrawCopy.limitSlot(maxMicros: 499_500_000), "Up to $499.50")
        XCTAssertEqual(WithdrawCopy.button(amountMicros: 120_000_000), "Withdraw $120")
        XCTAssertEqual(WithdrawCopy.button(amountMicros: 1_250_000), "Withdraw $1.25")
        XCTAssertEqual(WithdrawCopy.belowMinimum(minMicros: 1_000_000), "The minimum is $1")
    }

    func testTheSettledLineSaysAMinuteOrMinutes() {
        XCTAssertEqual(
            WithdrawCopy.settledLine(network: .solana, arrivalSeconds: 60),
            "Sent just now · arrives on Solana in about a minute")
        XCTAssertEqual(
            WithdrawCopy.settledLine(network: .base, arrivalSeconds: 90),
            "Sent just now · arrives on Base in about a minute")
        XCTAssertEqual(
            WithdrawCopy.settledLine(network: .base, arrivalSeconds: 120),
            "Sent just now · arrives on Base in about 2 minutes")
        XCTAssertEqual(
            WithdrawCopy.settledLine(network: .ethereum, arrivalSeconds: 300),
            "Sent just now · arrives on Ethereum in about 5 minutes")
    }

    func testTheArrivalNoteSaysAMinuteOrMinutes() {
        XCTAssertEqual(
            WithdrawCopy.arrivalNote(network: .solana, arrivalSeconds: 60), "Arrives on Solana in about a minute.")
        XCTAssertEqual(
            WithdrawCopy.arrivalNote(network: .base, arrivalSeconds: 120), "Arrives on Base in about 2 minutes.")
    }

    func testTheSettledCheckTruncatesTheAddress() {
        XCTAssertEqual(
            WithdrawCopy.settledCheck(
                amountMicros: 120_000_000, address: "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"),
            "$120 to 9xQe…4H6P")
    }

    func testNoStringSaysWallet() {
        var strings = [
            WithdrawCopy.assetLabel, WithdrawCopy.addAddress, WithdrawCopy.feeFailed, WithdrawCopy.invalidAddress,
            WithdrawCopy.ownDeposit, WithdrawCopy.emptyLine(feeMicros: nil, network: .base),
            WithdrawCopy.emptyLine(feeMicros: 0, network: .solana),
            WithdrawCopy.typedLine(receiveMicros: 1, feeMicros: 0),
            WithdrawCopy.limitLine(maxMicros: 1, overMicros: 1), WithdrawCopy.limitSlot(maxMicros: 1),
            WithdrawCopy.belowMinimum(minMicros: 1), WithdrawCopy.button(amountMicros: 1),
            WithdrawCopy.settledLine(network: .solana, arrivalSeconds: 60),
            WithdrawCopy.settledCheck(amountMicros: 1, address: "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"),
            WithdrawCopy.fullBalanceLabel(micros: 1),
        ]
        let solana: [WithdrawAddressProblem] = [
            .solana(.empty), .solana(.badCharacter("0")), .solana(.notAnAccountAddress), .solana(.ownDepositAddress),
        ]
        let evm: [WithdrawAddressProblem] = [.evm(.empty), .evm(.notAnAddress), .evm(.badChecksum)]
        for network in WithdrawNetwork.allCases {
            for problem in network.isEVM ? evm : solana {
                strings.append(WithdrawCopy.addressMessage(for: problem, network: network))
            }
        }
        for string in strings {
            XCTAssertFalse(string.localizedCaseInsensitiveContains("wallet"), string)
        }
    }

    func testTheToastMatchesTodaysForIdleSubmittedConfirmedAndFailed() {
        XCTAssertNil(WithdrawCopy.toast(for: .idle, fullBalanceLabel: nil))
        XCTAssertNil(WithdrawCopy.toast(for: .submitting, fullBalanceLabel: "$120 (full balance)"))
        XCTAssertEqual(
            WithdrawCopy.toast(for: .submitted(withdrawal(.submitted)), fullBalanceLabel: nil),
            WithdrawToast(
                message: "Withdrawing $120.00. It lands in about a minute.", isSuccess: false, link: nil))
        XCTAssertEqual(
            WithdrawCopy.toast(for: .failed(code: nil), fullBalanceLabel: nil),
            WithdrawToast(message: WithdrawProgress.didNotGoThrough, isSuccess: false, link: nil))
        XCTAssertEqual(
            WithdrawCopy.toast(for: .failed(code: "privy_unavailable"), fullBalanceLabel: "$120 (full balance)"),
            WithdrawToast(message: WithdrawProgress.didNotGoThrough, isSuccess: false, link: nil))
    }

    func testAConfirmedToastIsASuccessWithTheSolscanLink() throws {
        let toast = try XCTUnwrap(
            WithdrawCopy.toast(for: .confirmed(withdrawal(.confirmed, signature: signature)), fullBalanceLabel: nil))

        XCTAssertEqual(toast.message, "Withdrawal complete: $120.00")
        XCTAssertTrue(toast.isSuccess)
        let url = try XCTUnwrap(URL(string: "https://solscan.io/tx/\(signature)"))
        XCTAssertEqual(toast.link, .init(title: "View on Solscan", url: url))
        let bare = try XCTUnwrap(
            WithdrawCopy.toast(for: .confirmed(withdrawal(.confirmed, signature: nil)), fullBalanceLabel: nil))
        XCTAssertTrue(bare.isSuccess)
        XCTAssertNil(bare.link)
    }

    func testTheFullBalanceLabelReplacesTheAmountInTheToast() {
        let label = WithdrawCopy.fullBalanceLabel(micros: 120_000_000)

        XCTAssertEqual(label, "$120.00 (full balance)")
        XCTAssertEqual(
            WithdrawCopy.toast(for: .submitted(withdrawal(.submitted)), fullBalanceLabel: label)?.message,
            "Withdrawing $120.00 (full balance). It lands in about a minute.")
        XCTAssertEqual(
            WithdrawCopy.toast(for: .confirmed(withdrawal(.confirmed)), fullBalanceLabel: label)?.message,
            "Withdrawal complete: $120.00 (full balance)")
    }
}
