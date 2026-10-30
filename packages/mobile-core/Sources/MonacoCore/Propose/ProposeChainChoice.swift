import MonacoAPI

public enum ProposeChain: Equatable, Sendable {
    case best
    case network(Components.Schemas.Network)
    case unavailable(String)

    public var network: Components.Schemas.Network? {
        switch self {
        case .best, .unavailable: nil
        case .network(let network): network
        }
    }
}

public struct ProposeChainOption: Identifiable, Equatable, Sendable {
    public let chain: ProposeChain
    public let title: String
    public let figure: String
    public let networkID: String
    public let isBest: Bool
    public var isAvailable = true

    public var id: String {
        switch chain {
        case .best: "best"
        case .network(let network): network.rawValue
        case .unavailable(let id): id
        }
    }

    public static func options(_ bestPrice: BestPrice?, isEstimate: Bool = false) -> [ProposeChainOption] {
        guard let bestPrice, !bestPrice.isSell else { return [] }
        var networks: [Components.Schemas.Network] = []
        for venue in bestPrice.venues where !networks.contains(venue.network) && LaunchFeatures.offers(venue.network) {
            networks.append(venue.network)
        }
        guard networks.count > 1 else { return [] }
        let winner = bestPrice.winner
        let figure = { (venue: BestPrice.Venue) in
            isEstimate ? "about \(bestPrice.figure(venue))" : bestPrice.figure(venue)
        }
        let best = ProposeChainOption(
            chain: .best, title: "Best price", figure: figure(winner), networkID: winner.network.rawValue, isBest: true)
        let chains = networks.compactMap { network in
            bestPrice.venue(on: network).map {
                ProposeChainOption(
                    chain: .network(network), title: BestPrice.chainName(network), figure: figure($0),
                    networkID: network.rawValue, isBest: false)
            }
        }
        return [best] + chains
    }
}
