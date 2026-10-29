import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class GifSearchModel {
    public static let debounce: Duration = .milliseconds(300)
    public static let unavailableCopy = "GIFs aren’t available right now."

    public var query = "" {
        didSet {
            guard normalized != Self.normalize(oldValue) else { return }
            reload(after: Self.debounce)
        }
    }
    public private(set) var phase: LoadState<[Gif]> = .idle
    public private(set) var attribution = GifPage.giphyAttribution
    public private(set) var isLoadingMore = false

    private let gifs: any GifProvider
    private let clock: any Clock<Duration>
    private var nextOffset: Int?
    private var generation = 0
    private var task: Task<Void, Never>?

    public init(gifs: any GifProvider, clock: any Clock<Duration>) {
        self.gifs = gifs
        self.clock = clock
    }

    public var normalized: String { Self.normalize(query) }

    public var isSearching: Bool { !normalized.isEmpty }

    public var canLoadMore: Bool { nextOffset != nil && !isLoadingMore }

    public func load() {
        guard case .idle = phase else { return }
        reload(after: nil)
    }

    public func retry() {
        reload(after: nil)
    }

    public func loadMore() {
        guard case .loaded(let items) = phase, let offset = nextOffset, !isLoadingMore else { return }
        isLoadingMore = true
        let issued = generation
        let text = normalized
        task = Task { [weak self] in
            let result = await self?.fetch(text, offset: offset)
            guard let self, issued == generation, let result else { return }
            isLoadingMore = false
            if case .success(let page) = result {
                phase = .loaded(items + page.items.filter { gif in !items.contains { $0.id == gif.id } })
                nextOffset = page.nextOffset
            }
        }
    }

    public func select(_ gif: Gif) -> Components.Schemas.CommentAttachmentInput {
        .gif(
            .init(
                kind: .gif,
                gif: .init(
                    id: gif.id, title: gif.title, still: Self.input(gif.still), preview: Self.input(gif.preview),
                    original: Self.input(gif.original))))
    }

    private func reload(after delay: Duration?) {
        task?.cancel()
        generation += 1
        let issued = generation
        let text = normalized
        isLoadingMore = false
        nextOffset = nil
        phase = .loading
        task = Task { [weak self, clock] in
            if let delay {
                do { try await clock.sleep(for: delay) } catch { return }
            }
            let result = await self?.fetch(text, offset: 0)
            guard let self, issued == generation, let result else { return }
            switch result {
            case .success(let page):
                attribution = page.attribution
                nextOffset = page.nextOffset
                phase = .loaded(page.items)
            case .failure(let error):
                phase = .failed(error)
            }
        }
    }

    private func fetch(_ text: String, offset: Int) async -> Result<GifPage, APIError>? {
        do {
            if text.isEmpty { return .success(try await gifs.trending(offset: offset)) }
            return .success(try await gifs.search(text, offset: offset))
        } catch is CancellationError {
            return nil
        } catch {
            return .failure(APIError(error))
        }
    }

    private static func input(_ media: GifMedia) -> Components.Schemas.GifMedia {
        .init(url: media.url.absoluteString, width: media.width, height: media.height)
    }

    private static func normalize(_ text: String) -> String {
        text.trimmingCharacters(in: .whitespacesAndNewlines)
    }
}
