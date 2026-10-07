import Foundation
import MonacoAPI

public enum CabalJoinMode: String, CaseIterable, Identifiable, Sendable {
    case open
    case request

    public var id: String { rawValue }

    public var label: String {
        switch self {
        case .open: "Anyone"
        case .request: "I approve"
        }
    }

    public var caption: String {
        switch self {
        case .open: "Anyone can join, from search or with the invite code."
        case .request: "People ask to join, and you say yes or no."
        }
    }
}

public enum CabalVoterMode: String, CaseIterable, Identifiable, Sendable {
    case everyone = "all"
    case picked = "list"

    public var id: String { rawValue }

    public var label: String {
        switch self {
        case .everyone: "Everyone"
        case .picked: "People I pick"
        }
    }

    public var caption: String {
        switch self {
        case .everyone: "Every member votes on each proposal."
        case .picked: "Only the people you pick vote."
        }
    }
}

public enum CabalThreshold: String, CaseIterable, Identifiable, Sendable {
    case majority
    case unanimous

    public var id: String { rawValue }

    public var label: String {
        switch self {
        case .majority: "Majority"
        case .unanimous: "Everyone agrees"
        }
    }

    public var caption: String {
        switch self {
        case .majority: "Passes once more than half the voters say yes."
        case .unanimous: "Passes only if every voter says yes."
        }
    }
}

public enum CabalProposalExpiry: Int32, CaseIterable, Identifiable, Sendable {
    case oneHour = 3600
    case oneDay = 86_400
    case oneWeek = 604_800

    public var id: Int32 { rawValue }

    public var label: String {
        switch self {
        case .oneHour: "1 hour"
        case .oneDay: "1 day"
        case .oneWeek: "1 week"
        }
    }

    public var caption: String {
        "A vote that hasn't passed closes after \(label)."
    }
}

public struct CreateCabalInput: Equatable, Sendable {
    public let name: String
    public let joinMode: CabalJoinMode
    public let voterMode: CabalVoterMode
    public let threshold: CabalThreshold
    public let proposalExpirySeconds: Int32

    public init(
        name: String,
        joinMode: CabalJoinMode,
        voterMode: CabalVoterMode,
        threshold: CabalThreshold,
        proposalExpirySeconds: Int32
    ) {
        self.name = name
        self.joinMode = joinMode
        self.voterMode = voterMode
        self.threshold = threshold
        self.proposalExpirySeconds = proposalExpirySeconds
    }

    public var request: Components.Schemas.CreateCabalRequest {
        Components.Schemas.CreateCabalRequest(
            name: name,
            joinMode: joinMode.rawValue,
            voterMode: voterMode.rawValue,
            threshold: threshold.rawValue,
            proposalExpirySeconds: proposalExpirySeconds
        )
    }
}

public struct CreateCabalForm: Equatable, Sendable {
    public static let nameLength = 3...40

    public enum NameProblem: Equatable, Sendable {
        case empty
        case tooShort
        case tooLong
        case invalid

        public var message: String? {
            switch self {
            case .empty: nil
            case .tooShort: "Use at least 3 characters."
            case .tooLong: "Use 40 characters or fewer."
            case .invalid: "Remove tabs and line breaks."
            }
        }
    }

    public var name: String
    public var joinMode: CabalJoinMode
    public var voterMode: CabalVoterMode
    public var threshold: CabalThreshold
    public var expiry: CabalProposalExpiry

    public init(
        name: String = "",
        joinMode: CabalJoinMode = .open,
        voterMode: CabalVoterMode = .everyone,
        threshold: CabalThreshold = .majority,
        expiry: CabalProposalExpiry = .oneWeek
    ) {
        self.name = name
        self.joinMode = joinMode
        self.voterMode = voterMode
        self.threshold = threshold
        self.expiry = expiry
    }

    public var trimmedName: String {
        name.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    public var nameProblem: NameProblem? {
        let scalars = trimmedName.unicodeScalars
        if scalars.isEmpty { return .empty }
        if scalars.contains(where: { $0.properties.generalCategory == .control }) { return .invalid }
        if scalars.count < Self.nameLength.lowerBound { return .tooShort }
        if scalars.count > Self.nameLength.upperBound { return .tooLong }
        return nil
    }

    public var input: CreateCabalInput? {
        guard nameProblem == nil else { return nil }
        return CreateCabalInput(
            name: trimmedName,
            joinMode: joinMode,
            voterMode: voterMode,
            threshold: threshold,
            proposalExpirySeconds: expiry.rawValue
        )
    }
}
