import Foundation
import MonacoAPI

public enum WithdrawAddressProblem: Error, Equatable, Sendable {
    case solana(SolanaAddressProblem)
    case evm(EVMAddressProblem)

    public var isEmpty: Bool { self == .solana(.empty) || self == .evm(.empty) }
}

public enum WithdrawNetwork: String, CaseIterable, Identifiable, Sendable {
    case solana, base, arbitrum, ethereum, monad

    public var id: String { rawValue }

    public var name: String {
        switch self {
        case .solana: "Solana"
        case .base: "Base"
        case .arbitrum: "Arbitrum"
        case .ethereum: "Ethereum"
        case .monad: "Monad"
        }
    }

    public var isEVM: Bool { self != .solana }

    public var placeholder: String { "USDC address on \(name)" }

    public var withArticle: String { self == .arbitrum || self == .ethereum ? "an \(name)" : "a \(name)" }

    public var caveat: String {
        "\(withArticle.prefix(1).uppercased())\(withArticle.dropFirst()) address that accepts USDC. Transfers can't be undone."
    }

    public func validate(_ raw: String, ownDepositAddress: String?) -> Result<String, WithdrawAddressProblem> {
        if isEVM { return EVMAddress.validate(raw).mapError(WithdrawAddressProblem.evm) }
        return SolanaAddress.validate(raw, ownDepositAddress: ownDepositAddress).mapError(WithdrawAddressProblem.solana)
    }

    public func message(for problem: WithdrawAddressProblem) -> String {
        switch problem {
        case .solana(let problem): SolanaAddress.message(for: problem)
        case .evm(let problem): EVMAddress.message(for: problem, network: self)
        }
    }

    public func receiveLine(receiveMicros: Int64) -> String {
        let amount = UsdAmountFormatter.format(flooredMicros: receiveMicros)
        return isEVM ? "You'll receive ~\(amount)" : "You'll receive \(amount)"
    }

    func request(micros: Int64, toAddress: String) -> Components.Schemas.WithdrawRequest {
        Components.Schemas.WithdrawRequest(
            destinationKind: isEVM ? .evm : nil, network: isEVM ? .init(rawValue: rawValue) : nil,
            amountMicros: String(micros), toAddress: toAddress)
    }

    func previewRequest(micros: Int64, toAddress: String) -> Components.Schemas.WithdrawPreviewRequest {
        Components.Schemas.WithdrawPreviewRequest(
            destinationKind: isEVM ? .evm : nil, network: isEVM ? .init(rawValue: rawValue) : nil,
            amountMicros: String(micros), toAddress: toAddress)
    }
}
