import Testing

@testable import MonacoCore

struct AddressPayloadTests {
    private static let solana = "11111111111111111111111111111112"
    private static let recipient = "0x52908400098527886E0F7030069857D2E4169EE7"
    private static let usdcContract = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"

    private func parsed(_ text: String) -> (address: String, network: WithdrawNetwork?)? {
        AddressPayload.parse(text)
    }

    @Test func aBareSolanaAddressPicksSolana() {
        let result = parsed(Self.solana)
        #expect(result?.address == Self.solana)
        #expect(result?.network == .solana)
    }

    @Test func aSolanaURIDropsItsQuery() {
        let result = parsed("solana:\(Self.solana)?amount=1.5&label=Exchange&spl-token=EPjF")
        #expect(result?.address == Self.solana)
        #expect(result?.network == .solana)
    }

    @Test func aBareEVMAddressKeepsThePickedNetwork() {
        let result = parsed(Self.recipient)
        #expect(result?.address == Self.recipient)
        #expect(result?.network == nil)
    }

    @Test func aDirectEthereumURINamesItsChain() {
        let result = parsed("ethereum:\(Self.recipient)@8453?value=1e6")
        #expect(result?.address == Self.recipient)
        #expect(result?.network == .base)
    }

    @Test func aDirectEthereumURIWithNoChainKeepsThePickedNetwork() {
        let result = parsed("ethereum:\(Self.recipient)")
        #expect(result?.address == Self.recipient)
        #expect(result?.network == nil)
    }

    @Test(arguments: ["1", "10", "137", "42161", "0", "0x1", "abc", ""])
    func anUnsupportedChainIsRejected(chain: String) {
        #expect(parsed("ethereum:\(Self.recipient)@\(chain)") == nil)
    }

    @Test func anERC20TransferNeverYieldsTheContract() {
        let uri = "ethereum:\(Self.usdcContract)@8453/transfer?address=\(Self.recipient)&uint256=1000000"
        #expect(parsed(uri) == nil)
    }

    @Test func anyFunctionPathIsRejected() {
        #expect(parsed("ethereum:\(Self.usdcContract)@1/approve?address=\(Self.recipient)&uint256=1") == nil)
        #expect(parsed("ethereum:\(Self.recipient)/anything") == nil)
        #expect(parsed("ethereum:\(Self.recipient)@1/") == nil)
    }

    @Test func anInvalidAddressInsideAValidURIIsRejected() {
        #expect(parsed("solana:not-an-address?amount=1") == nil)
        #expect(parsed("ethereum:0x1234@1") == nil)
        #expect(parsed("ethereum:vitalik.eth@1") == nil)
        #expect(parsed("ethereum:0x52908400098527886E0F7030069857D2E4169ee7@1") == nil)
    }

    @Test func garbageAndEmptyAreRejected() {
        #expect(parsed("") == nil)
        #expect(parsed("   ") == nil)
        #expect(parsed("https://example.com") == nil)
        #expect(parsed("hello world") == nil)
        #expect(parsed("bitcoin:\(Self.solana)") == nil)
    }
}
