import Foundation
import Synchronization

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

final class MockURLProtocol: URLProtocol {
    private final class StoredHandler: @unchecked Sendable {
        let call: (URLRequest) throws -> (HTTPURLResponse, Data)

        init(_ call: @escaping (URLRequest) throws -> (HTTPURLResponse, Data)) {
            self.call = call
        }
    }

    private static let handlers = Mutex<StoredHandler?>(nil)

    static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))? {
        get { handlers.withLock { $0 }?.call }
        set {
            let stored = newValue.map(StoredHandler.init)
            handlers.withLock { $0 = stored }
        }
    }

    override class func canInit(with request: URLRequest) -> Bool {
        true
    }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        guard let handler = Self.handlers.withLock({ $0 })?.call else {
            client?.urlProtocol(
                self,
                didFailWithError: URLError(.badURL)
            )
            return
        }

        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}
