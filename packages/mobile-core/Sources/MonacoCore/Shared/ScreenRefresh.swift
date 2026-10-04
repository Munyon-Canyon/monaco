import Observation

@Observable
@MainActor
public final class ScreenRefresh {
    private var reloads: [String: @MainActor () async -> Void] = [:]

    public init() {}

    public func register(_ key: String, reload: @escaping @MainActor () async -> Void) {
        reloads[key] = reload
    }

    public func run() async {
        await withTaskGroup(of: Void.self) { group in
            for reload in reloads.values {
                group.addTask { await reload() }
            }
        }
    }
}
