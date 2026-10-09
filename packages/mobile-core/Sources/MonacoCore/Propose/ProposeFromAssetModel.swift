import MonacoAPI
import Observation

public enum ProposeKind: Hashable, Sendable {
    case buy, sell
}

@Observable @MainActor
public final class ProposeFromAssetModel {
    public enum Destination: Equatable {
        case pick
        case amount(String)
        case join
        case votersOnly
    }
    public private(set) var state: LoadState<[Components.Schemas.MyCabal]> = .idle
    public private(set) var memberOfAnyCabal = false
    private let api: APIClient

    public init(api: APIClient) { self.api = api }

    public func load(kind: ProposeKind, symbol: String) async {
        if case .loaded = state {} else { state = .loading }
        do {
            let mine = try await api.read { try await $0.getMyCabals().ok.body.json }
            var voting = mine.filter(\.canVote)
            if kind == .sell { voting = try await holding(voting, symbol: symbol) }
            memberOfAnyCabal = !mine.isEmpty
            state = .loaded(voting)
        } catch { state = .failed(APIError(error)) }
    }

    public var destination: Destination? {
        guard case .loaded(let cabals) = state else { return nil }
        if cabals.isEmpty { return memberOfAnyCabal ? .votersOnly : .join }
        if cabals.count == 1 { return .amount(cabals[0].id) }
        return .pick
    }

    private func holding(
        _ cabals: [Components.Schemas.MyCabal], symbol: String
    ) async throws -> [Components.Schemas.MyCabal] {
        let api = api
        let held = try await withThrowingTaskGroup(of: (String, Bool).self) { group in
            for cabal in cabals {
                group.addTask {
                    let pot = try await api.read {
                        try await $0.getCabalPot(path: .init(id: cabal.id)).ok.body.json
                    }
                    let holds = pot.holdings.contains { $0.symbol.caseInsensitiveCompare(symbol) == .orderedSame }
                    return (cabal.id, holds)
                }
            }
            var ids = Set<String>()
            for try await (id, holds) in group where holds { ids.insert(id) }
            return ids
        }
        return cabals.filter { held.contains($0.id) }
    }
}
