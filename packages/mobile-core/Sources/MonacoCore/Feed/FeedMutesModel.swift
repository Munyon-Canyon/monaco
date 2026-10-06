import MonacoAPI
import Observation

@Observable
@MainActor
public final class FeedMutesModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case empty
        case failed(APIError)
        case loaded([Components.Schemas.FeedMute])
    }

    public private(set) var phase = Phase.loading
    public private(set) var failureTick = 0
    public private(set) var lastError: APIError?

    private let service: FeedMuteService

    public init(api: APIClient) {
        service = FeedMuteService(api: api)
    }

    public var mutes: [Components.Schemas.FeedMute] {
        if case .loaded(let mutes) = phase { return mutes }
        return []
    }

    public func load() async {
        if case .failed = phase { phase = .loading }
        do {
            let mutes = try await service.list()
            phase = mutes.isEmpty ? .empty : .loaded(mutes)
        } catch {
            guard mutes.isEmpty else { return }
            phase = .failed(APIError(error))
        }
    }

    public static func label(_ mute: Components.Schemas.FeedMute) -> String {
        if let label = mute.label, !label.isEmpty { return label }
        return FeedKind(rawValue: mute.targetId)?.muteLabel ?? "this"
    }

    public func unmute(_ mute: Components.Schemas.FeedMute) async -> String? {
        guard let target = FeedMuteTarget(mute) else { return nil }
        do {
            try await service.unmute(target)
        } catch {
            lastError = APIError(error)
            failureTick += 1
            return nil
        }
        let rest = mutes.filter { $0.id != mute.id }
        phase = rest.isEmpty ? .empty : .loaded(rest)
        return "Unmuted \(Self.label(mute))."
    }
}
