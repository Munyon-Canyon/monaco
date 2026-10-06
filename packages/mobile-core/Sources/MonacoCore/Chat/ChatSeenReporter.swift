import Foundation
import MonacoAPI

public actor ChatSeenReporter {
    public static let window = Duration.seconds(2)

    private let cabalID: String
    private let api: APIClient
    private let clock: any Clock<Duration>
    private let makeKey: @Sendable () -> String
    private var screenVisible = false
    private var appActive = true
    private var cooldown: Task<Void, Never>?
    private var trailing = false

    public init(
        cabalID: String,
        api: APIClient,
        clock: any Clock<Duration>,
        makeKey: @escaping @Sendable () -> String = { UUID().uuidString.lowercased() }
    ) {
        self.cabalID = cabalID
        self.api = api
        self.clock = clock
        self.makeKey = makeKey
    }

    public func setScreenVisible(_ visible: Bool) async {
        let was = isEligible
        screenVisible = visible
        await settle(wasEligible: was)
    }

    public func setAppActive(_ active: Bool) async {
        let was = isEligible
        appActive = active
        await settle(wasEligible: was)
    }

    public func messageArrived() async {
        guard isEligible else { return }
        if cooldown == nil {
            report()
        } else {
            trailing = true
        }
    }

    public var isThrottled: Bool { cooldown != nil }

    private var isEligible: Bool { screenVisible && appActive }

    private func settle(wasEligible: Bool) async {
        if isEligible {
            if !wasEligible { report() }
        } else {
            trailing = false
        }
    }

    private func report() {
        trailing = false
        cooldown?.cancel()
        let clock = clock
        let task = Task { [weak self] in
            try? await clock.sleep(for: Self.window)
            guard !Task.isCancelled else { return }
            await self?.windowClosed()
        }
        cooldown = task
        let cabalID = cabalID
        let submission = IdempotentSubmission(makeKey: makeKey)
        let api = api
        Task {
            _ = try? await api.submit(
                submission, payload: Empty(), operation: "markChatSeen:\(cabalID)"
            ) { client, key in
                try await client.markChatSeen(
                    path: .init(id: cabalID), headers: .init(idempotencyKey: key)
                ).ok.body.json
            }
        }
    }

    private func windowClosed() async {
        cooldown = nil
        if trailing, isEligible { report() }
    }

    private struct Empty: Encodable, Sendable {}
}
