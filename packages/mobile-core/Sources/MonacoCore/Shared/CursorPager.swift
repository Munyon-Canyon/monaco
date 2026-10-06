import MonacoAPI
import Observation

public enum PagerPhase: Equatable, Sendable {
    case idle
    case loadingFirst
    case loadingMore
    case exhausted
    case failed(APIError)
}

@Observable
@MainActor
public final class CursorPager<Item: Identifiable & Sendable> {
    public private(set) var items: [Item] = []
    public private(set) var phase: PagerPhase = .idle

    private var nextCursor: String?
    private var listGeneration = 0
    private var loadingMore = false
    private let fetch: @Sendable (String?) async throws -> (items: [Item], nextCursor: String?)

    public init(
        fetch: @escaping @Sendable (_ cursor: String?) async throws -> (items: [Item], nextCursor: String?)
    ) {
        self.fetch = fetch
    }

    public func loadFirst() async {
        listGeneration += 1
        let mine = listGeneration
        loadingMore = false
        phase = .loadingFirst
        do {
            let page = try await fetch(nil)
            guard mine == listGeneration else { return }
            items = page.items
            nextCursor = page.nextCursor
            phase = settled(nextCursor)
        } catch {
            guard mine == listGeneration else { return }
            phase = .failed(APIError(error))
        }
    }

    public func loadMore() async {
        guard !loadingMore, phase != .loadingFirst, let cursor = nextCursor else { return }
        let mine = listGeneration
        loadingMore = true
        phase = .loadingMore
        do {
            let page = try await fetch(cursor)
            guard mine == listGeneration else { return }
            loadingMore = false
            appendDeduped(page.items)
            nextCursor = page.nextCursor
            phase = settled(nextCursor)
        } catch {
            guard mine == listGeneration else { return }
            loadingMore = false
            phase = .failed(APIError(error))
        }
    }

    public func refreshFirstPage() async {
        let mine = listGeneration
        do {
            let page = try await fetch(nil)
            guard mine == listGeneration, phase != .loadingFirst else { return }
            let pageIDs = Set(page.items.map(\.id))
            if !items.isEmpty && !items.contains(where: { pageIDs.contains($0.id) }) {
                items = page.items
                nextCursor = page.nextCursor
                listGeneration += 1
                loadingMore = false
                phase = settled(nextCursor)
                return
            }
            let tail = items.filter { !pageIDs.contains($0.id) }
            items = page.items + tail
            if tail.isEmpty && !loadingMore {
                nextCursor = page.nextCursor
            }
            if !loadingMore {
                phase = settled(nextCursor)
            }
        } catch {
            guard mine == listGeneration, phase != .loadingFirst else { return }
            if !loadingMore {
                phase = .failed(APIError(error))
            }
        }
    }

    public func remove(where shouldRemove: (Item) -> Bool) {
        items.removeAll(where: shouldRemove)
    }

    private func appendDeduped(_ page: [Item]) {
        var seen = Set(items.map(\.id))
        for item in page where seen.insert(item.id).inserted {
            items.append(item)
        }
    }

    private func settled(_ cursor: String?) -> PagerPhase {
        cursor == nil ? .exhausted : .idle
    }
}
