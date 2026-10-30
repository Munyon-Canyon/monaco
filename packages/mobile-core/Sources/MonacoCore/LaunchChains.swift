import MonacoAPI

extension LaunchFeatures {
    public static let chains: [Components.Schemas.Network] = [.solana, .base, .robinhood]
    public static let moneyChains: [Components.Schemas.Network] = [.solana, .base]

    public static func offers(_ network: String) -> Bool { chains.contains { $0.rawValue == network } }

    public static func offers(_ network: Components.Schemas.Network) -> Bool { chains.contains(network) }

    public static func offersMoney(_ network: String) -> Bool { moneyChains.contains { $0.rawValue == network } }
}
