import Foundation
import MonacoAPI
import MonacoFlows
import Observation

public enum ReportKind: String, Sendable, Hashable, CaseIterable {
    case message
    case comment
    case user
    case cabal
}

public enum ReportReason: String, Sendable, Hashable, CaseIterable, Identifiable {
    case spam
    case abuse
    case other

    public var id: String { rawValue }

    public var label: String {
        switch self {
        case .spam: "Spam"
        case .abuse: "Abusive"
        case .other: "Something else"
        }
    }
}

public enum Flow20aAction: Sendable, Equatable {
    case report
    case block(handle: String)
    case unblock(handle: String)
}

extension Notification.Name {
    public static let monacoBlocksChanged = Notification.Name("xyz.monacolabs.blocksChanged")
}

@Observable
@MainActor
public final class Flow20aReportBlockModel {
    public private(set) var lastFailure: APIError?

    private let api: APIClient
    private let hints: any HintSource
    private let reportSubmission = IdempotentSubmission()
    private let blockSubmission = IdempotentSubmission()
    private let unblockSubmission = IdempotentSubmission()

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public func report(kind: ReportKind, targetID: String, reason: ReportReason) async -> Flow20aOutcome {
        let call = ReportCall(kind: kind.rawValue, targetID: targetID, reason: reason.rawValue)
        return await settle {
            _ = try await api.submit(reportSubmission, payload: call, operation: "postReport") { client, key in
                try await client.postReport(
                    headers: .init(idempotencyKey: key),
                    body: .json(.init(kind: kind.schema, targetId: targetID, reason: reason.schema))
                ).created.body.json
            }
        }
    }

    public func block(userID: String) async -> Flow20aOutcome {
        let outcome = await settle {
            _ = try await api.submit(blockSubmission, payload: userID, operation: "postUserBlock") { client, key in
                try await client.postUserBlock(path: .init(id: userID), headers: .init(idempotencyKey: key))
                    .noContent
            }
        }
        if outcome == .ok { NotificationCenter.default.post(name: .monacoBlocksChanged, object: nil) }
        return outcome
    }

    public func unblock(userID: String) async -> Flow20aOutcome {
        let outcome = await settle {
            _ = try await api.submit(unblockSubmission, payload: userID, operation: "deleteUserBlock") { client, key in
                try await client.deleteUserBlock(path: .init(id: userID), headers: .init(idempotencyKey: key))
                    .noContent
            }
        }
        if outcome == .ok { NotificationCenter.default.post(name: .monacoBlocksChanged, object: nil) }
        return outcome
    }

    public func toast(for outcome: Flow20aOutcome, after action: Flow20aAction) -> String {
        switch outcome {
        case .ok:
            return Self.successToast(action)
        case .cannotBlockSelf, .userNotFound, .reportTargetNotFound, .rateLimited, .unauthorized, .interrupted:
            return lastFailure.map(ToastCopy.message(for:)) ?? ToastCopy.message(for: .transport(URLError(.unknown)))
        }
    }

    static func successToast(_ action: Flow20aAction) -> String {
        switch action {
        case .report:
            "Thanks. We'll take a look."
        case .block(let handle):
            handle.isEmpty
                ? "Blocked. You won't see their messages or comments."
                : "Blocked @\(handle). You won't see their messages or comments."
        case .unblock(let handle):
            handle.isEmpty ? "Unblocked." : "Unblocked @\(handle)."
        }
    }

    private func settle(_ call: () async throws -> Void) async -> Flow20aOutcome {
        do {
            try await call()
            lastFailure = nil
            return .ok
        } catch {
            let failure = APIError(error)
            lastFailure = failure
            return Flow20aOutcome(failure) ?? .interrupted
        }
    }
}

private struct ReportCall: Encodable, Sendable {
    let kind: String
    let targetID: String
    let reason: String
}

extension ReportKind {
    fileprivate var schema: Components.Schemas.ReportRequest.KindPayload {
        switch self {
        case .message: .message
        case .comment: .comment
        case .user: .user
        case .cabal: .cabal
        }
    }
}

extension ReportReason {
    fileprivate var schema: Components.Schemas.ReportRequest.ReasonPayload {
        switch self {
        case .spam: .spam
        case .abuse: .abuse
        case .other: .other
        }
    }
}
