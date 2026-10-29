import Foundation
import Observation

public enum MinBuildGateState: Equatable, Sendable {
    case ok
    case blocked(URL)
}

@Observable @MainActor
public final class MinBuildGate {
    public typealias Fetch = @Sendable () async throws -> (minIOSBuild: Int, storeURL: URL)

    public private(set) var state: MinBuildGateState = .ok

    private let fetch: Fetch
    private let currentBuild: Int

    public init(currentBuild: Int, fetch: @escaping Fetch) {
        self.currentBuild = currentBuild
        self.fetch = fetch
    }

    public func refresh() async {
        guard let config = try? await fetch() else {
            state = .ok
            return
        }
        state = config.minIOSBuild > 0 && currentBuild < config.minIOSBuild ? .blocked(config.storeURL) : .ok
    }
}
