import SwiftUI
import UIKit
import XCTest

@testable import Monaco

nonisolated private final class GifStillStubProtocol: URLProtocol {
    nonisolated override class func canInit(with request: URLRequest) -> Bool { true }

    nonisolated override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    nonisolated override func startLoading() {
        guard let url = request.url,
            let response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil, headerFields: nil)
        else { return }
        let renderer = UIGraphicsImageRenderer(size: CGSize(width: 40, height: 40))
        let data = renderer.pngData { context in
            UIColor.systemBlue.setFill()
            context.fill(CGRect(x: 0, y: 0, width: 40, height: 40))
        }
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: data)
        client?.urlProtocolDidFinishLoading(self)
    }

    nonisolated override func stopLoading() {}
}

nonisolated final class GifImageViewTests: XCTestCase {
    @MainActor
    func testInitialStill_returnsACachedStill() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [GifStillStubProtocol.self]
        let store = MonacoRemoteImageStore(session: URLSession(configuration: configuration), maxPixelSize: 480)
        let url = try XCTUnwrap(URL(string: "https://stills.test/primed.png"))

        let loaded = await store.image(for: url)

        XCTAssertNotNil(loaded)
        XCTAssertTrue(GifImageView.initialStill(for: url, store: store) === loaded)
    }

    @MainActor
    func testInitialStill_isNilForAnUnprimedURL() async throws {
        let store = MonacoRemoteImageStore(maxPixelSize: 480)
        let url = try XCTUnwrap(URL(string: "https://stills.test/unprimed.png"))

        XCTAssertNil(GifImageView.initialStill(for: url, store: store))
    }

    @MainActor
    func testBudget_releasingASlotLetsAWaitingTileClaimIt() {
        let budget = GifPlaybackBudget(limit: 1)
        XCTAssertTrue(budget.claim("a"))
        XCTAssertFalse(budget.claim("b"))
        let before = budget.revision

        budget.release("a")

        XCTAssertGreaterThan(budget.revision, before)
        XCTAssertTrue(budget.claim("b"))
    }
}
