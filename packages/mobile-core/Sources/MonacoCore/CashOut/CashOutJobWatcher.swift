import MonacoAPI
import Observation

@Observable
@MainActor
public final class CashOutJobWatcher {
    public static let changedHint = "cashout_changed"

    public private(set) var notice: CashOutNotice?
    private var running: [String: CashOutJob] = [:]

    private let api: APIClient
    private let hints: any HintSource
    @ObservationIgnored private var observer: Task<Void, Never>?

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public func job(for cabalID: String) -> CashOutJob? {
        running[cabalID]
    }

    public var isObserving: Bool { observer != nil }

    public func track(_ job: CashOutJob) {
        settle(job)
        guard job.isRunning, observer == nil else { return }
        let changes = hints.hints(matching: .user(what: Self.changedHint))
        observer = Task { [weak self] in
            await self?.refresh()
            if self?.running.isEmpty == false {
                for await _ in changes {
                    guard let self else { return }
                    await self.refresh()
                    if self.running.isEmpty { break }
                }
            }
            guard !Task.isCancelled else { return }
            self?.observer = nil
        }
    }

    public func reset() {
        observer?.cancel()
        observer = nil
        running = [:]
        notice = nil
    }

    private func refresh() async {
        for job in running.values {
            let read = try? await api.read { client in
                try await client.getCashOutJob(path: .init(id: job.cabalID, jobId: job.id)).ok.body.json
            }
            guard let read, running[job.cabalID]?.id == job.id else { continue }
            settle(CashOutJob(read))
        }
    }

    private func settle(_ job: CashOutJob) {
        if job.isRunning {
            running[job.cabalID] = job
        } else {
            running[job.cabalID] = nil
            notice = job.outcome
        }
    }
}
