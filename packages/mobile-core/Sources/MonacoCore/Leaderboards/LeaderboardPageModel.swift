import Foundation
import Observation

public enum LeaderboardBoardKind: String, CaseIterable, Hashable, Sendable {
    case cabals
    case people

    public var label: String {
        switch self {
        case .cabals: "Cabals"
        case .people: "People"
        }
    }

    var board: LeaderboardBoard {
        switch self {
        case .cabals: .cabals
        case .people: .people
        }
    }
}

public enum LeaderboardSort: CaseIterable, Hashable, Sendable {
    case returnPercent, pnlAmount, name

    public var label: String {
        switch self {
        case .returnPercent: "Return %"
        case .pnlAmount: "P&L amount"
        case .name: "Name A–Z"
        }
    }
}

public enum LeaderboardMembership: CaseIterable, Hashable, Sendable {
    case all, acceptingRequests, open, approval

    public var label: String {
        switch self {
        case .all: "All"
        case .acceptingRequests: "Accepting requests"
        case .open: "Open"
        case .approval: "Approval"
        }
    }
}

public enum LeaderboardMinimumMembers: CaseIterable, Hashable, Sendable {
    case any, five, ten

    public var label: String {
        switch self {
        case .any: "Any cabal size"
        case .five: "5+ members"
        case .ten: "10+ members"
        }
    }

    var floor: Int {
        switch self {
        case .any: 0
        case .five: 5
        case .ten: 10
        }
    }
}

public enum LeaderboardPeopleScope: CaseIterable, Hashable, Sendable {
    case everyone, following

    public var label: String {
        switch self {
        case .everyone: "Everyone"
        case .following: "People you follow"
        }
    }

    var filter: LeaderboardFilter {
        switch self {
        case .everyone: .everyone
        case .following: .friends
        }
    }
}

public struct LeaderboardFilters: Equatable, Sendable {
    public var sort = LeaderboardSort.returnPercent
    public var membership = LeaderboardMembership.all
    public var minimumMembers = LeaderboardMinimumMembers.any
    public var scope = LeaderboardPeopleScope.everyone

    public static let standard = LeaderboardFilters()

    public init() {}
}

public enum LeaderboardMovement: Equatable, Sendable {
    case up(Int)
    case down(Int)
    case unchanged
    case new
}

public struct LeaderboardAnnotation: Equatable, Sendable {
    public var movement: LeaderboardMovement?
    public var memberCount: Int?
    public var membership: LeaderboardMembership?

    public init(
        movement: LeaderboardMovement? = nil, memberCount: Int? = nil, membership: LeaderboardMembership? = nil
    ) {
        self.movement = movement
        self.memberCount = memberCount
        self.membership = membership
    }
}

public struct LeaderboardFixture: Sendable {
    public enum Phase: Sendable {
        case loaded, loading, failed
    }

    public var cabals: [Entry]
    public var people: [Entry]
    public var phase: Phase

    public struct Entry: Sendable {
        public var row: LeaderboardRowView
        public var annotation: LeaderboardAnnotation

        public init(row: LeaderboardRowView, annotation: LeaderboardAnnotation = LeaderboardAnnotation()) {
            self.row = row
            self.annotation = annotation
        }
    }

    public init(cabals: [Entry] = [], people: [Entry] = [], phase: Phase = .loaded) {
        self.cabals = cabals
        self.people = people
        self.phase = phase
    }
}

public struct LeaderboardPageRow: Identifiable, Equatable, Sendable {
    public let row: LeaderboardRowView
    public let movement: LeaderboardMovement?
    public let comparesMovement: Bool
    public let memberCount: Int?

    public init(row: LeaderboardRowView, movement: LeaderboardMovement?, comparesMovement: Bool, memberCount: Int?) {
        self.row = row
        self.movement = movement
        self.comparesMovement = comparesMovement
        self.memberCount = memberCount
    }

    public var id: String { row.id }

    public var subtitle: String? { memberCount.map(LeaderboardCopy.members) }

    public var glyphMovement: Int? {
        guard comparesMovement else { return 0 }
        switch movement {
        case .up(let places): return places
        case .down(let places): return -places
        case .unchanged: return 0
        case .new, nil: return nil
        }
    }
}

public enum LeaderboardPageState: Equatable, Sendable {
    case loading
    case rows
    case empty
    case filteredEmpty
    case stale
    case failed
}

@Observable
@MainActor
public final class LeaderboardPageModel {
    public private(set) var category: LeaderboardBoardKind
    public private(set) var range: LeaderboardRange
    public private(set) var appliedFilters: LeaderboardFilters
    public var draftFilters: LeaderboardFilters

    @ObservationIgnored private let fixture: LeaderboardFixture?
    @ObservationIgnored private let makeLoader:
        (@MainActor (LeaderboardBoard, LeaderboardRange, LeaderboardFilter) -> LeaderboardLoader)?
    private var loaders: [LeaderboardBoardKind: LeaderboardLoader] = [:]

    public init(
        board: LeaderboardBoardKind = .cabals, range: LeaderboardRange = .oneMonth,
        filters: LeaderboardFilters = .standard, fixture: LeaderboardFixture? = nil,
        makeLoader: (@MainActor (LeaderboardBoard, LeaderboardRange, LeaderboardFilter) -> LeaderboardLoader)? = nil
    ) {
        category = board
        self.range = range
        appliedFilters = filters
        draftFilters = filters
        self.fixture = fixture
        self.makeLoader = makeLoader
    }

    public var isFixture: Bool { fixture != nil }

    public var supportsFullFilters: Bool { isFixture }

    public var hasMovement: Bool { isFixture }

    public var hasFiltersApplied: Bool { appliedFilters != .standard }

    public var contextLine: String {
        LeaderboardCopy.contextLine(board: category, range: range, hasMovement: hasMovement)
    }

    public var toast: String? { liveLoader?.toast }

    public var liveLoader: LeaderboardLoader? {
        guard !isFixture, let makeLoader else { return nil }
        if let existing = loaders[category] { return existing }
        let created = makeLoader(category.board, range, category == .people ? appliedFilters.scope.filter : .everyone)
        loaders[category] = created
        return created
    }

    public var isLoadingMore: Bool { liveLoader?.isLoadingMore ?? false }
    public var hasMore: Bool { liveLoader?.hasMore ?? false }

    public var pinnedMe: LeaderboardRowView? { liveLoader?.pinnedMe }

    public var entries: [LeaderboardPageRow] {
        if let fixture {
            return shaped(category == .cabals ? fixture.cabals : fixture.people)
        }
        return (liveLoader?.rows ?? []).map {
            LeaderboardPageRow(row: $0, movement: nil, comparesMovement: true, memberCount: nil)
        }
    }

    public var pageState: LeaderboardPageState { state() }

    public var followingIsEmpty: Bool {
        category == .people && appliedFilters.scope == .following && pageState == .empty
    }

    public func activate() async {
        guard let loader = liveLoader else { return }
        await loader.load()
        await loader.observe()
    }

    public func reload() async {
        await liveLoader?.reload()
    }

    public func loadMore() async {
        await liveLoader?.loadMore()
    }

    public func retry() {
        liveLoader?.retry()
    }

    public func setVisible(_ visible: Bool) {
        liveLoader?.setVisible(visible)
    }

    public func dismissToast() {
        liveLoader?.dismissToast()
    }

    public func select(category next: LeaderboardBoardKind) {
        guard next != category else { return }
        category = next
        liveLoader?.select(range: range)
    }

    public func select(range next: LeaderboardRange) {
        guard next != range else { return }
        range = next
        liveLoader?.select(range: next)
    }

    public func beginEditingFilters() {
        draftFilters = appliedFilters
    }

    public func applyDraft() {
        commit(draftFilters)
    }

    public func discardDraft() {
        draftFilters = appliedFilters
    }

    public func resetFilters() {
        commit(.standard)
    }

    private func commit(_ filters: LeaderboardFilters) {
        appliedFilters = filters
        draftFilters = filters
        loaders[.people]?.select(filter: filters.scope.filter)
    }

    public static func isStale(_ rows: [LeaderboardRowView]) -> Bool {
        !rows.isEmpty && rows.allSatisfy(\.pricesDelayed)
    }

    private func state() -> LeaderboardPageState {
        let loaded: [LeaderboardRowView]
        if let fixture {
            switch fixture.phase {
            case .loading: return .loading
            case .failed: return .failed
            case .loaded: break
            }
            let source = category == .cabals ? fixture.cabals : fixture.people
            loaded = source.map(\.row)
            if loaded.isEmpty { return .empty }
        } else if let loader = liveLoader {
            switch loader.phase {
            case .loading: return .loading
            case .failed: return .failed
            case .empty: return emptyState
            case .loaded: loaded = loader.rows
            }
        } else {
            return .loading
        }
        if Self.isStale(loaded) { return .stale }
        return entries.isEmpty && !loaded.isEmpty ? .filteredEmpty : .rows
    }

    private var emptyState: LeaderboardPageState {
        var otherFilters = appliedFilters
        otherFilters.scope = .everyone
        return otherFilters == .standard ? .empty : .filteredEmpty
    }

    private func shaped(_ source: [LeaderboardFixture.Entry]) -> [LeaderboardPageRow] {
        var kept = source
        if category == .cabals {
            kept = kept.filter { entry in
                let size = entry.annotation.memberCount ?? 0
                let joins =
                    appliedFilters.membership == .all || entry.annotation.membership == appliedFilters.membership
                return size >= appliedFilters.minimumMembers.floor && joins
            }
        }
        switch appliedFilters.sort {
        case .returnPercent: break
        case .pnlAmount: kept.sort { $0.row.pnlMicros > $1.row.pnlMicros }
        case .name: kept.sort { $0.row.name.localizedCaseInsensitiveCompare($1.row.name) == .orderedAscending }
        }
        let compares = appliedFilters.sort == .returnPercent
        return kept.map {
            LeaderboardPageRow(
                row: $0.row, movement: $0.annotation.movement, comparesMovement: compares,
                memberCount: category == .cabals ? $0.annotation.memberCount : nil)
        }
    }
}
