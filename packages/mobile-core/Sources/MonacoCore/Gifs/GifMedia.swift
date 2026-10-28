import Foundation

public struct GifMedia: Sendable, Hashable {
    public let url: URL
    public let width: Int
    public let height: Int

    public init(url: URL, width: Int, height: Int) {
        self.url = url
        self.width = width
        self.height = height
    }
}
