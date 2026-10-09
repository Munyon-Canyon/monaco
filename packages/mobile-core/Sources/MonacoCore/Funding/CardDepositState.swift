import Foundation
import MonacoAPI
import Observation

public enum CardDepositState: Equatable, Sendable {
    case idle
    case creating
    case browsing(sessionID: String)
    case processing(sessionID: String)
    case done(sessionID: String)
    case failed(sessionID: String?, message: String)

    public enum Event: Equatable, Sendable {
        case start
        case created(sessionID: String)
        case createFailed(message: String)
        case redirected(sessionID: String)
        case status(sessionID: String, OnrampStatus)
        case deposited
    }

    public static let didNotGoThrough = "Card purchase didn't go through."

    public var sessionID: String? {
        switch self {
        case .idle, .creating: nil
        case .browsing(let id), .processing(let id), .done(let id): id
        case .failed(let id, _): id
        }
    }

    public func applying(_ event: Event) -> CardDepositState {
        switch (self, event) {
        case (.creating, .start):
            self
        case (_, .start):
            .creating
        case (.creating, .created(let id)):
            .browsing(sessionID: id)
        case (.creating, .createFailed(let message)):
            .failed(sessionID: nil, message: message)
        case (.done(let id), .redirected(let redirected)) where id == redirected,
            (.failed(.some(let id), _), .redirected(let redirected)) where id == redirected:
            self
        case (_, .redirected(let id)):
            .processing(sessionID: id)
        case (.browsing(let id), .status(let read, let status)) where id == read,
            (.processing(let id), .status(let read, let status)) where id == read:
            switch status {
            case .created, .opened: self
            case .confirmed, .submitted: .processing(sessionID: id)
            case .cancelled, .failed, .expired: .failed(sessionID: id, message: Self.didNotGoThrough)
            }
        case (.browsing(let id), .deposited), (.processing(let id), .deposited):
            .done(sessionID: id)
        default:
            self
        }
    }
}

public struct CardDepositPage: Identifiable, Equatable, Sendable {
    public let id: String
    public let url: URL
}

@Observable
@MainActor
public final class CardDeposit {
    public static let refreshingHint = "onramp_changed"
    public static let processingLine = "Card purchase processing"
    private static let processingWindow: TimeInterval = 3600

    public private(set) var state: CardDepositState = .idle
    public private(set) var page: CardDepositPage?
    public private(set) var message: String?
    public private(set) var messageTick = 0

    private let source: OnrampSource
    private let hints: any HintSource
    private let now: @Sendable () -> Date
    private var submission = IdempotentSubmission()
    private var checkOnForeground = false
    private var processingSince: Date?

    public init(source: OnrampSource, hints: any HintSource, now: @escaping @Sendable () -> Date) {
        self.source = source
        self.hints = hints
        self.now = now
    }

    public var isCreating: Bool { state == .creating }

    public var isProcessing: Bool {
        guard case .processing = state, let since = processingSince else { return false }
        return !expired(since)
    }

    public func start(suggestedMicros: Int64?, cabalID: String?) async {
        guard !isCreating else { return }
        apply(.start)
        do {
            let session = try await source.createSession(
                suggestedMicros: suggestedMicros, cabalID: cabalID, submission: submission)
            submission = IdempotentSubmission()
            apply(.created(sessionID: session.sessionID))
            if state == .browsing(sessionID: session.sessionID) {
                page = CardDepositPage(id: session.sessionID, url: session.url)
            }
        } catch {
            apply(.createFailed(message: ToastCopy.message(for: APIError(error))))
        }
    }

    public func browserClosed() {
        guard page != nil else { return }
        page = nil
        if case .browsing = state { checkOnForeground = true }
    }

    public func redirected(sessionID: String) async {
        apply(.redirected(sessionID: sessionID))
        await readStatus(of: sessionID)
    }

    public func foregrounded() async {
        await checkDeposited()
        guard checkOnForeground, page == nil, case .browsing(let id) = state else { return }
        checkOnForeground = false
        await readStatus(of: id)
    }

    public func balanceChanged(_ change: BalanceChange?) {
        guard case .deposited = change else { return }
        apply(.deposited)
    }

    public func observe() async {
        async let onramp: Void = followOnramp()
        async let balance: Void = followBalance()
        await checkDeposited()
        _ = await (onramp, balance)
    }

    public func reset() {
        state = .idle
        processingSince = nil
        page = nil
        message = nil
        checkOnForeground = false
        submission = IdempotentSubmission()
    }

    private var liveSessionID: String? {
        switch state {
        case .browsing(let id), .processing(let id): id
        default: nil
        }
    }

    private func followOnramp() async {
        for await _ in hints.hints(matching: .user(what: Self.refreshingHint)) {
            guard let id = liveSessionID else { continue }
            await readStatus(of: id)
        }
    }

    private func followBalance() async {
        for await _ in hints.hints(matching: .user(what: BalanceSource.refreshingHint)) {
            await checkDeposited()
        }
    }

    private func expired(_ since: Date) -> Bool {
        now().timeIntervalSince(since) > Self.processingWindow
    }

    private func checkDeposited() async {
        guard case .processing = state, let since = processingSince else { return }
        if expired(since) {
            reset()
            return
        }
        guard let landed = try? await source.latestDepositAt(), landed >= since, processingSince == since else {
            return
        }
        apply(.deposited)
    }

    private func readStatus(of id: String) async {
        guard let status = try? await source.session(id: id) else { return }
        apply(.status(sessionID: id, status))
    }

    private func apply(_ event: CardDepositState.Event) {
        let next = state.applying(event)
        guard next != state else { return }
        state = next
        if case .processing = next { processingSince = now() } else { processingSince = nil }
        if case .browsing = next {} else { page = nil }
        if case .failed(_, let text) = next {
            message = text
            messageTick += 1
        }
    }
}
