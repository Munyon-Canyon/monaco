import Foundation
import MonacoAPI
import Observation

public struct MemberActivityRow: Equatable, Sendable, Identifiable {
    public enum Glyph: Equatable, Sendable {
        case voted
        case proposed
        case replied
        case toppedBoard

        public var symbol: String {
            switch self {
            case .voted: "checkmark.circle"
            case .proposed: "arrow.up.right"
            case .replied: "bubble.left"
            case .toppedBoard: "crown"
            }
        }
    }

    public enum Destination: Equatable, Sendable {
        case proposal(String)
        case board
    }

    public let id: String
    public let glyph: Glyph
    public let title: String
    public let subtitle: String
    public let time: String
    public let destination: Destination

    public init(
        id: String, glyph: Glyph, title: String, subtitle: String, time: String, destination: Destination
    ) {
        self.id = id
        self.glyph = glyph
        self.title = title
        self.subtitle = subtitle
        self.time = time
        self.destination = destination
    }
}

@Observable
@MainActor
public final class MemberActivityModel {
    public static let pageSize = 20

    public let userID: String
    public private(set) var state: LoadState<[MemberActivityRow]> = .idle
    public private(set) var isLoadingMore = false
    public private(set) var hasMore = false

    private let api: APIClient
    private let now: @Sendable () -> Date
    private var cursor: String?
    private var generation = 0

    public init(userID: String, api: APIClient, now: @escaping @Sendable () -> Date = PortfolioClock.system) {
        self.userID = userID
        self.api = api
        self.now = now
    }

    public var rows: [MemberActivityRow] {
        if case .loaded(let rows) = state { return rows }
        return []
    }

    public func load() async {
        generation += 1
        let issued = generation
        if case .loaded = state {
        } else {
            state = .loading
        }
        do {
            let page = try await read(cursor: nil)
            guard issued == generation else { return }
            cursor = page.nextCursor
            hasMore = page.nextCursor != nil
            state = .loaded(page.items.enumerated().map { Self.row($1, index: $0, now: now()) })
        } catch {
            guard issued == generation else { return }
            if case .loaded = state { return }
            state = .failed(APIError(error))
        }
    }

    public func loadMore() async {
        guard case .loaded(let loaded) = state, let next = cursor, !isLoadingMore else { return }
        isLoadingMore = true
        defer { isLoadingMore = false }
        let issued = generation
        guard let page = try? await read(cursor: next), issued == generation else { return }
        cursor = page.nextCursor
        hasMore = page.nextCursor != nil
        let more = page.items.enumerated().map { Self.row($1, index: loaded.count + $0, now: now()) }
        state = .loaded(loaded + more)
    }

    private func read(cursor: String?) async throws -> Components.Schemas.UserActivityPage {
        let userID = userID
        return try await api.read { client in
            try await client.getUserActivity(
                path: .init(id: userID), query: .init(cursor: cursor, limit: Self.pageSize)
            ).ok.body.json
        }
    }

    private static func row(_ activity: Components.Schemas.UserActivity, index: Int, now: Date) -> MemberActivityRow {
        switch activity {
        case .voted(let vote):
            let side = vote.choice.map { $0 == .yes ? "Voted Yes" : "Voted No" }
            return MemberActivityRow(
                id: "\(index)", glyph: .voted, title: side ?? "Voted",
                subtitle: "\(vote.subject) · \(vote.cabal.name)", time: time(vote.at, now),
                destination: .proposal(vote.proposalId))
        case .proposed(let proposal):
            return MemberActivityRow(
                id: "\(index)", glyph: .proposed, title: "Proposed \(proposal.subject)",
                subtitle: "\(proposal.cabal.name) · \(outcome(proposal))", time: time(proposal.at, now),
                destination: .proposal(proposal.proposalId))
        case .replied(let reply):
            let amount = reply.usdcMicros.map { " " + UsdAmountFormatter.format(micros: $0) } ?? ""
            return MemberActivityRow(
                id: "\(index)", glyph: .replied, title: "“\(reply.body)”",
                subtitle: "on \(reply.subject)\(amount) · \(reply.cabal.name)", time: time(reply.at, now),
                destination: .proposal(reply.proposalId))
        case .toppedBoard(let win):
            return MemberActivityRow(
                id: "\(index)", glyph: .toppedBoard, title: "Topped the board",
                subtitle: "\(month(win.periodStart)) · \(win.right) of \(win.resolved) right",
                time: time(win.at, now), destination: .board)
        }
    }

    private static func outcome(_ proposal: Components.Schemas.UserActivityProposed) -> String {
        guard let result = proposal.result else { return "open" }
        let verdict = ["passed", "executed", "execution_blocked"].contains(proposal.status) ? "passed" : "failed"
        return "\(verdict) \(result.yes)–\(result.no)"
    }

    private static let months = [
        "January", "February", "March", "April", "May", "June", "July", "August", "September", "October",
        "November", "December",
    ]

    private static func month(_ periodStart: String) -> String {
        let parts = periodStart.split(separator: "-")
        guard parts.count >= 2, let number = Int(parts[1]), months.indices.contains(number - 1) else {
            return "A month"
        }
        return months[number - 1]
    }

    static func time(_ date: Date, _ now: Date) -> String {
        let elapsed = max(0, now.timeIntervalSince(date))
        switch elapsed {
        case ..<60: return "now"
        case ..<3600: return "\(Int(elapsed / 60))m"
        case ..<86_400: return "\(Int(elapsed / 3600))h"
        case ..<604_800: return "\(Int(elapsed / 86_400))d"
        default: return "\(Int(elapsed / 604_800))w"
        }
    }
}
