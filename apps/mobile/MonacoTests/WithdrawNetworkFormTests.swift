import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct WithdrawNetworkFormTests {
    private let evmAddress = "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
    private let solanaAddress = "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"

    private var balance: AccountBalance {
        AccountBalance(
            availableMicros: 20_000_000, onChainMicros: 20_000_000, inFlightMicros: 0, depositAddress: "dep",
            asOf: Date())
    }

    private func form(
        _ network: WithdrawNetwork, address: String, amount: String = "10", estimate: Int64? = nil
    ) -> WithdrawForm {
        WithdrawForm(
            amountText: amount, destinationAddress: address, network: network, estimatedReceiveMicros: estimate,
            balance: balance)
    }

    @Test func thePickerOffersSolanaBaseArbitrumAndEthereum() {
        #expect(WithdrawNetwork.allCases.map(\.name) == ["Solana", "Base", "Arbitrum", "Ethereum"])
    }

    @Test func eachEVMNetworkTakesAnEVMAddressAndRefusesASolanaOne() {
        for network in [WithdrawNetwork.base, .arbitrum, .ethereum] {
            #expect(form(network, address: evmAddress).canContinue)
            #expect(form(network, address: evmAddress).addressProblem == nil)
            let refused = form(network, address: solanaAddress)
            #expect(!refused.canContinue)
            #expect(
                refused.addressProblem
                    == "Check the address. It doesn't look like \(network.withArticle) address."
            )
            #expect(refused.addressCaveat == network.caveat)
        }
    }

    @Test func aMistypedChecksumIsCaughtBeforeContinue() {
        let typo = form(.base, address: "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD")
        #expect(!typo.canContinue)
        #expect(typo.addressProblem == "That address has a typo in it. Copy it again.")
    }

    @Test func solanaKeepsItsOwnAddressRules() {
        #expect(form(.solana, address: solanaAddress).canContinue)
        #expect(!form(.solana, address: evmAddress).canContinue)
        #expect(form(.solana, address: "").addressProblem == nil)
        #expect(form(.base, address: "").addressProblem == nil)
    }

    @Test func theEstimateShowsWhatTheWalletReceives() {
        #expect(form(.base, address: evmAddress, estimate: 9_992_500).receiveLine == "You'll receive ~$9.99")
        #expect(form(.solana, address: solanaAddress).receiveLine == "You'll receive $10.00")
        #expect(form(.base, address: evmAddress).receiveLine == nil)
        #expect(form(.base, address: evmAddress, amount: "50", estimate: 49_000_000).receiveLine == nil)
    }

    @Test func confirmNamesTheNetwork() {
        let view = WithdrawConfirmView(
            destinationAddress: evmAddress, network: .arbitrum, amountText: "10", amountLabel: "$10.00",
            isSubmitting: false, onWithdraw: {})
        #expect(view.network.name == "Arbitrum")
    }
}
