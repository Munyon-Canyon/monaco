import MonacoAPI
import Observation

@Observable @MainActor
public final class ProposeFromAssetModel {
    public enum Destination: Equatable {
        case pick
        case amount(String)
        case join
    }
    public private(set) var state: LoadState<[Components.Schemas.MyCabal]> = .idle
    private let api: APIClient

    public init(api: APIClient) { self.api = api }

    public func load() async {
        state = .loading
        do {
            let cabals = try await api.read { try await $0.getMyCabals().ok.body.json }.filter(\.canVote)
            state = .loaded(cabals)
        } catch { state = .failed(APIError(error)) }
    }

    public var destination: Destination? {
        guard case .loaded(let cabals) = state else { return nil }
        if cabals.isEmpty { return .join }
        if cabals.count == 1 { return .amount(cabals[0].id) }
        return .pick
    }
}
