import Foundation

public struct ProposalSides: Equatable, Sendable {
    public struct Entry: Equatable, Identifiable, Sendable {
        public let id: String
        public let name: String
        public let photoURL: URL?
        public let isYou: Bool

        public init(id: String, name: String, photoURL: URL?, isYou: Bool) {
            self.id = id
            self.name = name
            self.photoURL = photoURL
            self.isYou = isYou
        }

        var shown: String { isYou ? "you" : name }
    }

    public enum Side: Equatable, Sendable {
        case yes, no, voted, notVoted
    }

    public struct Row: Equatable, Identifiable, Sendable {
        public let entry: Entry
        public let side: Side
        public var id: String { entry.id }
    }

    public let yes: [Entry]
    public let no: [Entry]
    public let notVoted: [Entry]
    public let votedHidden: [Entry]
    public let sidesVisible: Bool

    private static let nameLimit = 5

    public static func make(
        voters: [ProposalVoter], members: [ProposalMember], viewerID: String, sidesVisible: Bool
    ) -> ProposalSides {
        let byID = Dictionary(members.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
        var yes: [(Entry, Date?)] = []
        var no: [(Entry, Date?)] = []
        var hidden: [(Entry, Date?)] = []
        var missing: [(Entry, Date?)] = []
        for voter in voters {
            let member = byID[voter.id]
            let name = voter.displayName.isEmpty ? (member?.name ?? "Member") : voter.displayName
            let entry = Entry(
                id: voter.id, name: name, photoURL: voter.photoURL ?? member?.photoURL, isYou: voter.id == viewerID)
            let pair = (entry, voter.castAt)
            guard voter.voted else {
                missing.append(pair)
                continue
            }
            switch voter.ballot {
            case "yes" where sidesVisible || entry.isYou: yes.append(pair)
            case "no" where sidesVisible || entry.isYou: no.append(pair)
            default: hidden.append(pair)
            }
        }
        return ProposalSides(
            yes: ordered(yes), no: ordered(no), notVoted: ordered(missing), votedHidden: ordered(hidden),
            sidesVisible: sidesVisible)
    }

    private static func ordered(_ pairs: [(Entry, Date?)]) -> [Entry] {
        pairs.sorted { lhs, rhs in
            if lhs.0.isYou != rhs.0.isYou { return rhs.0.isYou }
            if let left = lhs.1, let right = rhs.1, left != right { return left < right }
            if (lhs.1 == nil) != (rhs.1 == nil) { return lhs.1 != nil }
            return lhs.0.name.localizedCaseInsensitiveCompare(rhs.0.name) == .orderedAscending
        }.map(\.0)
    }

    public var votedCount: Int { yes.count + no.count + votedHidden.count }

    public var strip: String {
        sidesVisible
            ? "\(yes.count) yes · \(no.count) no · \(notVoted.count) not voted"
            : "\(votedCount) voted · \(notVoted.count) not voted"
    }

    public var rows: [Row] {
        yes.map { Row(entry: $0, side: .yes) } + no.map { Row(entry: $0, side: .no) }
            + votedHidden.map { Row(entry: $0, side: .voted) } + notVoted.map { Row(entry: $0, side: .notVoted) }
    }

    public func entries(for side: Side) -> [Entry] {
        switch side {
        case .yes: yes
        case .no: no
        case .voted: votedHidden
        case .notVoted: notVoted
        }
    }

    public func line(for side: Side) -> String {
        let entries = entries(for: side)
        guard !entries.isEmpty else { return "" }
        let suffix = side == .notVoted ? " did not vote" : ""
        if entries.count > Self.nameLimit {
            let first = entries.prefix(Self.nameLimit - 1).map(\.shown).joined(separator: ", ")
            return "\(first) and \(entries.count - Self.nameLimit + 1) more\(suffix)"
        }
        let names = entries.map(\.shown)
        if side == .notVoted, names.count > 1, let last = names.last {
            return names.dropLast().joined(separator: ", ") + " and \(last)\(suffix)"
        }
        return names.joined(separator: ", ") + suffix
    }

    public func spokenLine(for side: Side) -> String {
        let entries = entries(for: side)
        let label =
            switch side {
            case .yes: "Yes"
            case .no: "No"
            case .voted: "Voted"
            case .notVoted: "Did not vote"
            }
        if entries.count > 1, entries.count <= Self.nameLimit, let last = entries.last {
            return "\(label): " + entries.dropLast().map(\.shown).joined(separator: ", ") + " and \(last.shown)"
        }
        return "\(label): \(line(for: side).replacingOccurrences(of: " did not vote", with: ""))"
    }
}
