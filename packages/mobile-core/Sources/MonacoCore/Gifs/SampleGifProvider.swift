import Foundation

public struct SampleGifProvider: GifProvider {
    public enum Mode: Sendable {
        case results
        case noMatches
        case failing
    }

    public static let host = "gifs.sample.monaco.test"

    private static let shapes: [(width: Int, height: Int)] = [
        (200, 200), (200, 112), (200, 300), (200, 150),
        (200, 267), (200, 100), (200, 250), (200, 180),
    ]

    public let mode: Mode

    public init(mode: Mode = .results) {
        self.mode = mode
    }

    public static var items: [Gif] {
        shapes.enumerated().compactMap { index, shape in
            let id = "sample-gif-\(index + 1)"
            guard let still = media(id, "still.png", shape),
                let preview = media(id, "preview.gif", shape),
                let original = media(id, "original.gif", shape)
            else { return nil }
            return Gif(id: id, title: "Sample GIF \(index + 1)", still: still, preview: preview, original: original)
        }
    }

    public func trending(offset: Int) async throws -> GifPage {
        if mode == .failing { throw GifProviderError.failed }
        return page(offset: offset)
    }

    public func search(_ query: String, offset: Int) async throws -> GifPage {
        switch mode {
        case .failing: throw GifProviderError.failed
        case .noMatches: return GifPage(items: [], nextOffset: nil)
        case .results: return page(offset: offset)
        }
    }

    private func page(offset: Int) -> GifPage {
        GifPage(items: offset == 0 ? Self.items : [], nextOffset: nil)
    }

    private static func media(_ id: String, _ file: String, _ shape: (width: Int, height: Int)) -> GifMedia? {
        URL(string: "https://\(host)/\(id)/\(file)").map {
            GifMedia(url: $0, width: shape.width, height: shape.height)
        }
    }
}
