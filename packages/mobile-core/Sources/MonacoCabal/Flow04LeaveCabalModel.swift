import Foundation
import MonacoAPI
import MonacoFlows
import Observation

public struct LeaveStanding: Sendable, Equatable {
    public let cabalName: String
    public let canLeave: Bool

    public init(cabalName: String, canLeave: Bool) {
        self.cabalName = cabalName
        self.canLeave = canLeave
    }

    init(_ cabal: Components.Schemas.MyCabal) {
        self.init(cabalName: cabal.name, canLeave: cabal.role != "creator" || cabal.memberCount <= 1)
    }
}

public enum LeaveOutcome: Sendable, Equatable {
    case left(cabalName: String)
    case cashOutFirst(message: String)
    case refused(message: String)

    public init(refusal error: APIError) {
        let message = ToastCopy.message(for: error)
        switch Flow04Outcome(error) {
        case .leaveHoldsShares:
            self = .cashOutFirst(message: message)
        case .leaveLastMemberPotNotEmpty, .leaveCreatorWithMembers, .notCabalMember, .unauthorized,
            .priceUnavailable, .ok, .interrupted, nil:
            self = .refused(message: message)
        }
    }
}

@Observable
@MainActor
public final class LeaveCabalModel {
    public private(set) var standing: LoadState<LeaveStanding?> = .idle
    public private(set) var isLeaving = false

    private let api: APIClient
    private let hints: any HintSource
    private let cabalID: String
    private let submission = IdempotentSubmission()
    private var generation = 0

    public init(api: APIClient, hints: any HintSource, cabalID: String) {
        self.api = api
        self.hints = hints
        self.cabalID = cabalID
    }

    public func load() async {
        generation += 1
        let mine = generation
        if case .idle = standing {
            standing = .loading
        }
        do {
            let cabals = try await api.read { client in
                try await client.getMyCabals().ok.body.json
            }
            guard mine == generation else { return }
            standing = .loaded(cabals.first { $0.id == cabalID }.map(LeaveStanding.init))
        } catch {
            guard mine == generation else { return }
            standing = .failed(APIError(error))
        }
    }

    public func observe() async {
        for await _ in hints.hints(matching: .cabal(id: cabalID, what: "members")) {
            if Task.isCancelled { return }
            await load()
        }
    }

    public func leave() async -> LeaveOutcome? {
        guard !isLeaving, case .loaded(let standing?) = standing, standing.canLeave else { return nil }
        isLeaving = true
        defer { isLeaving = false }
        let cabalID = cabalID
        do {
            _ = try await api.submit(submission, payload: cabalID, operation: "deleteCabalMemberMe") { client, key in
                try await client.deleteCabalMemberMe(
                    path: .init(id: cabalID),
                    headers: .init(idempotencyKey: key)
                ).noContent
            }
            return .left(cabalName: standing.cabalName)
        } catch {
            return LeaveOutcome(refusal: APIError(error))
        }
    }
}
