import Foundation
import MonacoAPI

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

final class StallingURLProtocol: URLProtocol {
    private final class Gate: @unchecked Sendable {
        let lock = NSLock()
        var held: [ObjectIdentifier: StallingURLProtocol] = [:]
        var writesReleased = false
    }

    private static let gate = Gate()

    static func releaseKeyedWrites() {
        gate.lock.lock()
        gate.writesReleased = true
        let ready = gate.held.filter { isKeyedWrite($0.value.request) }
        for id in ready.keys { gate.held[id] = nil }
        gate.lock.unlock()
        for proto in ready.values { proto.finishOK() }
    }

    static func reset() {
        gate.lock.lock()
        gate.writesReleased = false
        gate.held.removeAll()
        gate.lock.unlock()
    }

    override class func canInit(with request: URLRequest) -> Bool { true }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        Self.gate.lock.lock()
        if Self.gate.writesReleased, Self.isKeyedWrite(request) {
            Self.gate.lock.unlock()
            finishOK()
            return
        }
        Self.gate.held[ObjectIdentifier(self)] = self
        Self.gate.lock.unlock()
    }

    override func stopLoading() {
        Self.gate.lock.lock()
        Self.gate.held[ObjectIdentifier(self)] = nil
        Self.gate.lock.unlock()
    }

    private static func isKeyedWrite(_ request: URLRequest) -> Bool {
        request.value(forHTTPHeaderField: IdempotentSubmission.keyHeader) != nil
    }

    private func finishOK() {
        guard let url = request.url,
            let response = HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil, headerFields: nil)
        else { return }
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data("{}".utf8))
        client?.urlProtocolDidFinishLoading(self)
    }
}
