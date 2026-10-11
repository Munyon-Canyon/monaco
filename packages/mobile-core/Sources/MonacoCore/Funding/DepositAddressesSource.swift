import Foundation
import MonacoAPI
import Observation

public struct DepositNetwork: Equatable, Hashable, Sendable, Identifiable {
    public let id: String
    public let displayName: String
    public let address: String

    public init(id: String, address: String) {
        self.id = id
        self.displayName = Self.displayName(for: id)
        self.address = address
    }

    public static let solanaID = "solana"

    public var isSolana: Bool { id == Self.solanaID }

    public var isEVM: Bool { address.lowercased().hasPrefix("0x") }

    static func displayName(for id: String) -> String {
        switch id {
        case "solana": "Solana"
        case "base": "Base"
        case "arbitrum": "Arbitrum"
        case "ethereum": "Ethereum"
        case "robinhood": "Robinhood Chain"
        default: id.prefix(1).uppercased() + id.dropFirst()
        }
    }

    public static func list(json: Data) throws -> [DepositNetwork] {
        let wire: Wire
        do {
            wire = try JSONDecoder().decode(Wire.self, from: json)
        } catch {
            throw APIError.decoding("deposit addresses")
        }
        let networks = wire.addresses.compactMap { item -> DepositNetwork? in
            let id = item.network.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            let address = item.address.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !id.isEmpty, !address.isEmpty, item.asset == nil || item.asset == "USDC" else { return nil }
            return DepositNetwork(id: id, address: address)
        }
        return networks.filter(\.isSolana) + networks.filter { !$0.isSolana }
    }

    private struct Wire: Decodable {
        let addresses: [WireAddress]
    }
}

private struct WireAddress: Decodable {
    let network: String
    let address: String
    let asset: String?
}

@Observable
@MainActor
public final class DepositAddressesSource {
    public private(set) var state: LoadState<[DepositNetwork]> = .idle
    private let api: APIClient
    private var generation = 0

    public init(api: APIClient) {
        self.api = api
    }

    public var networks: [DepositNetwork]? {
        if case .loaded(let networks) = state { return networks }
        return nil
    }

    public func load() async {
        generation += 1
        let issued = generation
        if networks == nil { state = .loading }
        do {
            let data = try await api.sessionBody { client in
                _ = try await client.getMyDepositAddresses().ok
            }
            let fresh = try DepositNetwork.list(json: data)
            guard issued == generation else { return }
            state = .loaded(fresh)
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            state = .failed(APIError(error))
        }
    }

    public func reset() {
        generation += 1
        state = .idle
    }

    public static func offered(_ state: LoadState<[DepositNetwork]>, solanaAddress: String?) -> [DepositNetwork] {
        if case .loaded(let networks) = state, !networks.isEmpty { return networks }
        return [DepositNetwork(id: DepositNetwork.solanaID, address: solanaAddress ?? "")]
    }
}
