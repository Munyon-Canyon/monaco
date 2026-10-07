public struct FirstLaunchMarker {
    public static let key = "monaco.launchedBefore"

    private let store: any KeyValueStoring

    public init(store: any KeyValueStoring) {
        self.store = store
    }

    public func registerLaunch() -> Bool {
        let isFirst = !store.bool(forKey: Self.key)
        store.set(true, forKey: Self.key)
        return isFirst
    }
}
