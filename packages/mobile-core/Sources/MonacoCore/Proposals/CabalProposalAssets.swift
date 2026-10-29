import Observation

@Observable
@MainActor
public final class CabalProposalAssets {
    public private(set) var byTicker: [String: ProposalAsset] = [:]

    public init() {}

    public func load(symbols: [String], repository: ProposalsRepository) async {
        for symbol in Set(symbols) where byTicker[symbol] == nil {
            if let asset = try? await repository.asset(symbol: symbol) { byTicker[symbol] = asset }
        }
    }
}
