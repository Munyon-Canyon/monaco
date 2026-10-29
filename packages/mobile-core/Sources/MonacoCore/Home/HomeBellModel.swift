import MonacoAPI
import Observation

@Observable
@MainActor
public final class HomeBellModel {
    public static let largestShown = 9

    public private(set) var count = 0

    private let api: APIClient
    private let hints: any HintSource
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public var badgeText: String? {
        guard count > 0 else { return nil }
        return count > Self.largestShown ? "\(Self.largestShown)+" : "\(count)"
    }

    public var accessibilityLabel: String {
        count > 0 ? "Catch up, \(count) new" : "Catch up"
    }

    public func load() async {
        guard let fetched = try? await api.read({ try await $0.getMyCatchUpCount().ok.body.json }) else { return }
        guard !Task.isCancelled else { return }
        count = fetched.count
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .user(what: "catch_up_changed")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
