import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class ChatOpenVotesModel {
    public typealias Page = (items: [ProposalSummary], nextCursor: String?)
    public typealias Fetch = @Sendable (String?) async throws -> Page

    public static let sectionTitle = "Open votes"
    static let maxPages = 5

    public private(set) var votes: [ProposalSummary] = []

    private let cabalID: String
    private let fetch: Fetch
    @ObservationIgnored private var generation = 0

    public init(cabalID: String, fetch: @escaping Fetch) {
        self.cabalID = cabalID
        self.fetch = fetch
    }

    public convenience init(cabalID: String, api: APIClient) {
        let repository = ProposalsRepository(api: api)
        self.init(cabalID: cabalID) { cursor in
            try await repository.list(cabalID: cabalID, filter: .open, cursor: cursor)
        }
    }

    public var count: Int { votes.count }

    public func load() async {
        generation += 1
        let mine = generation
        var collected: [ProposalSummary] = []
        var cursor: String?
        for _ in 0..<Self.maxPages {
            guard let page = try? await fetch(cursor) else { return }
            collected += page.items
            guard let next = page.nextCursor else { break }
            cursor = next
        }
        guard mine == generation else { return }
        votes = Self.ordered(collected)
    }

    public func reload() async {
        await load()
    }

    public func observe(hints: any HintSource) async {
        for await _ in hints.hints(matching: .cabal(id: cabalID, what: "proposal_updated")) {
            await load()
        }
    }

    public func lead(hiding visible: Set<String>) -> ProposalSummary? {
        votes.first { !visible.contains($0.id) }
    }

    public static func capsuleLine(for vote: ProposalSummary, now: Date) -> String {
        let left = CountdownText(vote.expiresAt.timeIntervalSince(now))
        let parts: [String?] = [voted(vote), replies(vote.replyCount), left.compact]
        return parts.compactMap { $0 }.joined(separator: " · ")
    }

    public static func spokenLine(for vote: ProposalSummary, now: Date) -> String {
        let left = CountdownText(vote.expiresAt.timeIntervalSince(now))
        let parts: [String?] = [voted(vote), replies(vote.replyCount), "closes in \(left.spokenLeft)"]
        return parts.compactMap { $0 }.joined(separator: ", ")
    }

    private static func voted(_ vote: ProposalSummary) -> String {
        "\(vote.tally.yes + vote.tally.no) of \(vote.tally.voters) voted"
    }

    private static func replies(_ count: Int) -> String? {
        guard count > 0 else { return nil }
        return count == 1 ? "1 reply" : "\(count) replies"
    }

    private static func ordered(_ items: [ProposalSummary]) -> [ProposalSummary] {
        var seen: Set<String> = []
        return items.filter { $0.status == .open && seen.insert($0.id).inserted }
            .sorted { lhs, rhs in
                if lhs.expiresAt != rhs.expiresAt { return lhs.expiresAt < rhs.expiresAt }
                if lhs.createdAt != rhs.createdAt { return lhs.createdAt < rhs.createdAt }
                return lhs.id < rhs.id
            }
    }
}
