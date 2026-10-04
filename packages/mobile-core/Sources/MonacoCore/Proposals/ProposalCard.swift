import Foundation
import MonacoAPI

public struct ProposalPerson: Equatable, Sendable {
    public let userID: String
    public let name: String
    public let photoURL: String?

    init(_ member: Components.Schemas.CabalMember) {
        userID = member.userId
        photoURL = member.photoUrl
        let display = member.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        name = !display.isEmpty ? display : member.handle.map { "@\($0)" } ?? "Someone"
    }

    init(unknown userID: String) {
        self.userID = userID
        name = "Someone"
        photoURL = nil
    }
}

public struct ProposalCabal: Equatable, Sendable {
    public let id: String
    public let isMember: Bool
    public let canVote: Bool
    let people: [String: ProposalPerson]

    init(_ cabal: Components.Schemas.Cabal) {
        id = cabal.id
        isMember = cabal.me != nil
        canVote = cabal.me?.canVote ?? false
        people = Dictionary(
            cabal.members.map { ($0.userId, ProposalPerson($0)) }, uniquingKeysWith: { first, _ in first })
    }

    public func person(_ userID: String) -> ProposalPerson {
        people[userID] ?? ProposalPerson(unknown: userID)
    }
}

public enum ProposalBallot: Equatable, Sendable {
    case none
    case ask
    case voted(BallotChoice)

    init(status: ProposalStatus, myBallot: BallotChoice?, canVote: Bool) {
        guard status == .open, canVote else {
            self = .none
            return
        }
        self = myBallot.map(Self.voted) ?? .ask
    }
}

public struct ProposalCard: Identifiable, Equatable, Sendable {
    public let proposal: Proposal
    public let asset: ProposalAsset
    public let proposer: ProposalPerson
    public let ballot: ProposalBallot
    public let swap: SwapState?

    public var id: String { proposal.id }

    public init(proposal: Proposal, asset: ProposalAsset, proposer: ProposalPerson, canVote: Bool, swap: SwapState?) {
        self.proposal = proposal
        self.asset = asset
        self.proposer = proposer
        self.swap = swap
        ballot = ProposalBallot(status: proposal.status, myBallot: proposal.myBallot, canVote: canVote)
    }

    public var chip: String? {
        ProposalChip.label(status: proposal.status, kind: proposal.kind, swap: swap)
    }

    public func closes(now: Date) -> String? {
        proposal.status == .open ? ProposalCardCopy.closes(at: proposal.expiresAt, now: now) : nil
    }

    public var amount: String { asset.amount(of: proposal) }

    public var tracker: String {
        let tally = proposal.tally
        return ProposalCardCopy.tracker(voted: tally.voted, voters: tally.voters, needed: tally.needed)
    }

    public var awaitsViewer: Bool { ballot == .ask }
}
