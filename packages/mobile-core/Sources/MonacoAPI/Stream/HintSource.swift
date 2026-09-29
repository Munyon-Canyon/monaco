/// Where features get live hints. `HintStream` is the app's one connection; tests pass a fake.
public protocol HintSource: Sendable {
    func hints(matching filter: HintFilter) -> AsyncStream<Hint>
}
