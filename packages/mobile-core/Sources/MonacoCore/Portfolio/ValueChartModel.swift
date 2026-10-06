import MonacoAPI
import Observation

@Observable
@MainActor
public final class ValueChartModel {
    public private(set) var state: LoadState<[PnLHistoryLoader.Subject: ValueCurve]> = .idle
    public private(set) var range: LeaderboardRange
    public private(set) var toast: String?
    public let ranges: [LeaderboardRange]

    private let loader: PnLHistoryLoader
    private var subjects: [PnLHistoryLoader.Subject]
    private var shownRange: LeaderboardRange?
    private var generation = 0

    public init(
        subjects: [PnLHistoryLoader.Subject], ranges: [LeaderboardRange], range: LeaderboardRange, api: APIClient,
        hints: any HintSource
    ) {
        self.subjects = subjects
        self.ranges = ranges
        self.range = range
        loader = PnLHistoryLoader(api: api, hints: hints)
    }

    public var curves: [PnLHistoryLoader.Subject: ValueCurve]? {
        if case .loaded(let curves) = state { return curves }
        return nil
    }

    public var curve: ValueCurve? {
        subjects.first.flatMap { curves?[$0] }
    }

    public func load() async {
        await show(range)
    }

    public func select(_ next: LeaderboardRange) async {
        guard ranges.contains(next) else { return }
        range = next
        await show(next)
    }

    public func setSubjects(_ next: [PnLHistoryLoader.Subject]) async {
        if next == subjects {
            if case .failed = state { await show(range) }
            return
        }
        subjects = next
        shownRange = nil
        state = .idle
        await show(range)
    }

    public func observe() async {
        await loader.observe { [weak self] update in await self?.apply(update) }
    }

    public func setVisible(_ visible: Bool) {
        Task { await loader.setVisible(visible) }
    }

    public func dismissToast() {
        toast = nil
    }

    private func show(_ wanted: LeaderboardRange) async {
        generation += 1
        let issued = generation
        if curves == nil { state = .loading }
        do {
            guard let curves = try await loader.show(subjects, range: wanted), issued == generation else { return }
            state = .loaded(curves)
            shownRange = wanted
        } catch {
            guard issued == generation, !Task.isCancelled else { return }
            fail(APIError(error))
        }
    }

    private func apply(_ update: PnLHistoryLoader.Update) {
        switch update {
        case .refreshed(let refreshed, let curves):
            guard refreshed == range else { return }
            state = .loaded(curves)
            shownRange = refreshed
        case .failed(let error):
            fail(error)
        }
    }

    private func fail(_ error: APIError) {
        if curves == nil {
            state = .failed(error)
        } else {
            toast = ToastCopy.message(for: error)
            if let shownRange { range = shownRange }
        }
    }
}
