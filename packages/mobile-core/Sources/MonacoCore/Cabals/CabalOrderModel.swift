import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalOrderModel {
    public private(set) var order: [String] = []
    public private(set) var lastError: APIError?
    public private(set) var failureTick = 0
    public private(set) var isSaving = false
    private let api: APIClient
    private let submission = IdempotentSubmission()
    private let saved: @MainActor () async -> Void

    public init(api: APIClient, saved: @escaping @MainActor () async -> Void = {}) {
        self.api = api
        self.saved = saved
    }

    public func sync(_ ids: [String]) {
        guard !isSaving else { return }
        order = ids
    }

    public func move(_ id: String, before target: String) async {
        guard id != target, let from = order.firstIndex(of: id), let to = order.firstIndex(of: target) else {
            return
        }
        var next = order
        next.remove(at: from)
        next.insert(id, at: to)
        await save(next)
    }

    public func moveEarlier(_ id: String) async {
        guard let index = order.firstIndex(of: id), index > 0 else { return }
        await move(id, before: order[index - 1])
    }

    public func moveLater(_ id: String) async {
        guard let index = order.firstIndex(of: id), index < order.count - 1 else { return }
        var next = order
        next.swapAt(index, index + 1)
        await save(next)
    }

    private func save(_ next: [String]) async {
        guard !isSaving else { return }
        let previous = order
        order = next
        isSaving = true
        defer { isSaving = false }
        do {
            _ = try await api.submit(submission, payload: next, operation: "putMyCabalOrder") { client, key in
                try await client.putMyCabalOrder(
                    headers: .init(idempotencyKey: key),
                    body: .json(.init(cabalIds: next))
                ).noContent
            }
            await saved()
        } catch {
            order = previous
            lastError = APIError(error)
            failureTick += 1
        }
    }
}
