import Foundation
@preconcurrency import GiphyUISDK
import MonacoCore

nonisolated struct GiphyGifProvider: GifProvider {
    static let limit = 25
    static let rating = GPHRatingType.ratedPG13

    private let apiKey: String

    init(apiKey: String) {
        self.apiKey = apiKey
    }

    func trending(offset: Int) async throws -> GifPage {
        try await load(path: "gifs/trending", query: [], offset: offset)
    }

    func search(_ query: String, offset: Int) async throws -> GifPage {
        let term = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !term.isEmpty else { return try await trending(offset: offset) }
        return try await load(path: "gifs/search", query: [URLQueryItem(name: "q", value: term)], offset: offset)
    }

    static func config(apiKey: String, path: String, query: [URLQueryItem], offset: Int) -> GPHRequestConfig {
        let config = GPHRequestConfig()
        config.base = "https://api.giphy.com/v1/"
        config.method = .get
        config.path = path
        config.apiKey = apiKey
        config.queryItems =
            [
                URLQueryItem(name: "limit", value: String(limit)),
                URLQueryItem(name: "offset", value: String(max(offset, 0))),
                URLQueryItem(name: "rating", value: rating.rawValue),
            ] + query
        return config
    }

    static func page(from json: [String: Any], offset: Int) -> GifPage {
        let rows = json["data"] as? [[String: Any]] ?? []
        let media = rows.compactMap(decode)
        let taken = offset + rows.count
        let total = (json["pagination"] as? [String: Any])?["total_count"] as? Int ?? 0
        let next = rows.isEmpty || taken >= total ? nil : taken
        return GifPage(items: media.compactMap(gif), nextOffset: next)
    }

    static func gif(from media: GPHMedia) -> Gif? {
        guard let images = media.images,
            let still = mediaFile(images.fixedWidthStill),
            let preview = mediaFile(images.fixedWidth),
            let original = mediaFile(images.downsized) ?? mediaFile(images.original)
        else { return nil }
        return Gif(id: media.id, title: media.title ?? "", still: still, preview: preview, original: original)
    }

    static func isGiphyCDN(_ url: URL) -> Bool {
        guard url.scheme == "https", let host = url.host?.lowercased() else { return false }
        if host == "i.giphy.com" { return true }
        let labels = host.split(separator: ".", omittingEmptySubsequences: false)
        return labels.count == 3 && labels[0].hasPrefix("media") && labels[1] == "giphy" && labels[2] == "com"
    }

    private static func decode(_ row: [String: Any]) -> GPHMedia? {
        guard JSONSerialization.isValidJSONObject(row),
            let data = try? JSONSerialization.data(withJSONObject: row)
        else { return nil }
        return try? JSONDecoder().decode(GPHMedia.self, from: data)
    }

    private static func mediaFile(_ image: GPHImage?) -> GifMedia? {
        guard let image, let string = image.gifUrl, let url = URL(string: string), isGiphyCDN(url),
            image.width > 0, image.height > 0
        else { return nil }
        return GifMedia(url: url, width: image.width, height: image.height)
    }

    private func load(path: String, query: [URLQueryItem], offset: Int) async throws -> GifPage {
        guard !apiKey.isEmpty else { throw GifProviderError.unavailable }
        let config = Self.config(apiKey: apiKey, path: path, query: query, offset: offset)
        return try await withCheckedThrowingContinuation { continuation in
            GiphyCore.shared.httpRequest(with: config) { json, response, error in
                let status = (response as? HTTPURLResponse)?.statusCode ?? 0
                if error == nil, (200..<300).contains(status), let json {
                    continuation.resume(returning: Self.page(from: json, offset: offset))
                } else {
                    continuation.resume(throwing: GifProviderError.failed)
                }
            }
        }
    }
}
