import Foundation

public enum AddressPayload {
    private static let chains: [String: WithdrawNetwork] = ["1": .ethereum, "8453": .base, "42161": .arbitrum]

    public static func parse(_ text: String) -> (address: String, network: WithdrawNetwork?)? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if let rest = body(of: trimmed, scheme: "solana") {
            return solana(rest)
        }
        if let rest = body(of: trimmed, scheme: "ethereum") {
            return ethereum(rest)
        }
        if let address = solana(trimmed)?.address { return (address, .solana) }
        if let address = evm(trimmed) { return (address, nil) }
        return nil
    }

    private static func body(of text: String, scheme: String) -> String? {
        let prefix = scheme + ":"
        guard text.lowercased().hasPrefix(prefix) else { return nil }
        let rest = String(text.dropFirst(prefix.count))
        return rest.prefix { $0 != "?" && $0 != "#" }.description
    }

    private static func solana(_ raw: String) -> (address: String, network: WithdrawNetwork?)? {
        guard case .success(let address) = SolanaAddress.validate(raw) else { return nil }
        return (address, .solana)
    }

    private static func evm(_ raw: String) -> String? {
        guard case .success(let address) = EVMAddress.validate(raw) else { return nil }
        return address
    }

    private static func ethereum(_ target: String) -> (address: String, network: WithdrawNetwork?)? {
        guard !target.contains("/") else { return nil }
        let parts = target.split(separator: "@", maxSplits: 1, omittingEmptySubsequences: false)
        guard let address = evm(String(parts[0])) else { return nil }
        guard parts.count == 2 else { return (address, nil) }
        guard let network = chains[String(parts[1])], LaunchFeatures.offersMoney(network.rawValue) else { return nil }
        return (address, network)
    }
}
