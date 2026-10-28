import Foundation

public struct Gif: Sendable, Hashable, Identifiable {
    public let id: String
    public let title: String
    public let still: GifMedia
    public let preview: GifMedia
    public let original: GifMedia

    public init(id: String, title: String, still: GifMedia, preview: GifMedia, original: GifMedia) {
        self.id = id
        self.title = title
        self.still = still
        self.preview = preview
        self.original = original
    }
}

public struct GifPage: Sendable {
    public static let giphyAttribution = "Powered by GIPHY"

    public let items: [Gif]
    public let nextOffset: Int?
    public let attribution: String

    public init(items: [Gif], nextOffset: Int?, attribution: String = GifPage.giphyAttribution) {
        self.items = items
        self.nextOffset = nextOffset
        self.attribution = attribution
    }
}

public enum GifProviderError: Error, Equatable, Sendable {
    case unavailable
    case failed
}

public protocol GifProvider: Sendable {
    func trending(offset: Int) async throws -> GifPage
    func search(_ query: String, offset: Int) async throws -> GifPage
}

public struct UnavailableGifProvider: GifProvider {
    public init() {}

    public func trending(offset: Int) async throws -> GifPage {
        throw GifProviderError.unavailable
    }

    public func search(_ query: String, offset: Int) async throws -> GifPage {
        throw GifProviderError.unavailable
    }
}
