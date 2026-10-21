import Foundation
import MonacoAPI

public enum TxExplorer {
    public struct Link: Equatable, Sendable {
        public let name: String
        public let url: URL

        public init(name: String, url: URL) {
            self.name = name
            self.url = url
        }
    }

    public static func solscanURL(signature: String) -> URL? {
        guard !isEVMHash(signature) else { return nil }
        return URL(string: "https://solscan.io/tx/\(signature)")
    }

    public static func link(signature: String, network: Components.Schemas.Network?) -> Link? {
        guard isEVMHash(signature) else {
            return solscanURL(signature: signature).map { Link(name: "Solscan", url: $0) }
        }
        let host: (name: String, domain: String)? =
            switch network {
            case .base: ("Basescan", "basescan.org")
            case .ethereum: ("Etherscan", "etherscan.io")
            case .arbitrum: ("Arbiscan", "arbiscan.io")
            case .solana, .robinhood, nil: nil
            }
        return host.flatMap { host in
            URL(string: "https://\(host.domain)/tx/\(signature)").map { Link(name: host.name, url: $0) }
        }
    }

    private static func isEVMHash(_ signature: String) -> Bool { signature.hasPrefix("0x") }
}
