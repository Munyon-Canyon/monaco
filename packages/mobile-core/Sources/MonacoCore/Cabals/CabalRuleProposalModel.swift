import Foundation
import MonacoAPI
import Observation
import OpenAPIRuntime

@Observable
@MainActor
public final class CabalRuleProposalModel: Identifiable {
    public nonisolated var id: ObjectIdentifier { ObjectIdentifier(self) }

    public enum Change: Equatable, Sendable {
        case showSides(Bool)
        case threshold(CabalThreshold)
        case window(CabalProposalExpiry)
        case maxTrade(Int32?)
        case voter(userID: String, canVote: Bool)
        case admin(userID: String, isAdmin: Bool)
        case creator(userID: String)
    }

    public enum Refusal: Equatable, Sendable {
        case cannotPropose
        case targetsCreator
        case notAMember
        case notAVoter
        case onlyCreator
        case notAnAdmin
        case lastAdmin
        case adminMustVote
        case unchanged

        public var message: String {
            switch self {
            case .cannotPropose: "Voters propose rule changes."
            case .targetsCreator: "The creator's role can't change."
            case .notAMember: "They aren't in the cabal."
            case .notAVoter: "Make them a voter first."
            case .onlyCreator: "Only the creator can pass the creator role."
            case .notAnAdmin: "Make them an admin first."
            case .lastAdmin: "A cabal needs an admin."
            case .adminMustVote: "Admins always vote."
            case .unchanged: "That's already the rule."
            }
        }
    }

    public enum Outcome: Equatable, Sendable {
        case proposed(proposalID: String)
        case alreadyOpen
        case refused(Refusal)
        case failed(String)

        public var toast: String {
            switch self {
            case .proposed: "Proposed. Your cabal votes on it."
            case .alreadyOpen: "A vote on this rule is already open."
            case .refused(let refusal): refusal.message
            case .failed(let message): message
            }
        }
    }

    public static let thesisLimit = 280
    public static let maxTradeChoices: [Int32?] = [nil, 1_000, 2_000, 3_000, 4_000, 5_000]

    public let cabal: Components.Schemas.Cabal
    public var change: Change
    public var thesis = "" {
        didSet {
            if thesis.count > Self.thesisLimit { thesis = String(thesis.prefix(Self.thesisLimit)) }
        }
    }
    public private(set) var isSubmitting = false

    private let api: APIClient
    private let submission = IdempotentSubmission()

    public init(cabal: Components.Schemas.Cabal, change: Change, api: APIClient) {
        self.cabal = cabal
        self.change = change
        self.api = api
    }

    public var canPropose: Bool { cabal.me?.canPropose == true }

    public static func offersPassCreator(
        _ cabal: Components.Schemas.Cabal, to row: Components.Schemas.CabalMemberRow
    ) -> Bool {
        cabal.me?.role == "creator" && cabal.me?.canPropose == true && row.role == .admin
    }

    public var passLine: String {
        "Your cabal votes on it like a trade: \(CabalRulesRows.votesToPass(cabal)) to pass."
    }

    public var refusal: Refusal? {
        guard canPropose else { return .cannotPropose }
        switch change {
        case .showSides(let on): return on == cabal.rules.showSides ? .unchanged : nil
        case .threshold(let value): return value.rawValue == cabal.rules.threshold ? .unchanged : nil
        case .window(let value): return value.rawValue == cabal.rules.proposalExpirySeconds ? .unchanged : nil
        case .maxTrade(let bps): return bps == cabal.rules.maxTradeBps ? .unchanged : nil
        case .voter(let userID, let canVote): return voterRefusal(userID, canVote)
        case .admin(let userID, let isAdmin): return adminRefusal(userID, isAdmin)
        case .creator(let userID): return creatorRefusal(userID)
        }
    }

    public var canSubmit: Bool { refusal == nil && !isSubmitting }

    public func propose() async -> Outcome {
        if let refusal { return .refused(refusal) }
        guard !isSubmitting else { return .failed(ToastCopy.message(for: .inFlight)) }
        isSubmitting = true
        defer { isSubmitting = false }
        let request = requestBody()
        let cabalID = cabal.id
        do {
            let proposal = try await api.submit(
                submission, payload: request, operation: Operations.PostCabalRuleProposal.id
            ) { client, key in
                try await client.postCabalRuleProposal(
                    path: .init(id: cabalID), headers: .init(idempotencyKey: key), body: .json(request)
                ).created.body.json
            }
            return .proposed(proposalID: proposal.id)
        } catch {
            let error = APIError(error)
            if case .problem(let problem) = error, problem.code == .known(.ruleChangeOpen) {
                return .alreadyOpen
            }
            return .failed(ToastCopy.message(for: error))
        }
    }

    func requestBody() -> Components.Schemas.RuleChangeRequest {
        let trimmed = thesis.trimmingCharacters(in: .whitespacesAndNewlines)
        return Components.Schemas.RuleChangeRequest(
            change: wire(), thesis: trimmed.isEmpty ? nil : trimmed)
    }

    private func wire() -> Components.Schemas.RuleChangeRequest.ChangePayload {
        switch change {
        case .showSides(let on): .init(rule: .showSides, to: .init(booleanLiteral: on))
        case .threshold(let value): .init(rule: .threshold, to: .init(stringLiteral: value.rawValue))
        case .window(let value): .init(rule: .proposalExpirySeconds, to: .init(integerLiteral: Int(value.rawValue)))
        case .maxTrade(let bps):
            .init(rule: .maxTradeBps, to: bps.map { .init(integerLiteral: Int($0)) } ?? .init(nilLiteral: ()))
        case .voter(let userID, let canVote): .init(rule: .voter, to: .init(booleanLiteral: canVote), userId: userID)
        case .admin(let userID, let isAdmin): .init(rule: .admin, to: .init(booleanLiteral: isAdmin), userId: userID)
        case .creator(let userID): .init(rule: .creator, to: .init(booleanLiteral: true), userId: userID)
        }
    }

    private func voterRefusal(_ userID: String, _ canVote: Bool) -> Refusal? {
        if userID == cabal.creator.userId { return .targetsCreator }
        guard let member = cabal.members.first(where: { $0.userId == userID }) else { return .notAMember }
        if !canVote, isAdmin(userID) { return .adminMustVote }
        return member.canVote == canVote ? .unchanged : nil
    }

    private func adminRefusal(_ userID: String, _ makeAdmin: Bool) -> Refusal? {
        if userID == cabal.creator.userId { return .targetsCreator }
        guard let member = cabal.members.first(where: { $0.userId == userID }) else { return .notAMember }
        if makeAdmin, !member.canVote { return .notAVoter }
        if !makeAdmin, isAdmin(userID), (cabal.admins ?? []).count <= 1 { return .lastAdmin }
        return isAdmin(userID) == makeAdmin ? .unchanged : nil
    }

    private func creatorRefusal(_ userID: String) -> Refusal? {
        if cabal.me?.role != "creator" { return .onlyCreator }
        if userID == cabal.creator.userId { return .targetsCreator }
        guard cabal.members.contains(where: { $0.userId == userID }) else { return .notAMember }
        return isAdmin(userID) ? nil : .notAnAdmin
    }

    private func isAdmin(_ userID: String) -> Bool {
        (cabal.admins ?? []).contains { $0.userId == userID }
    }
}
