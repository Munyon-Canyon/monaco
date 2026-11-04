import Foundation
import Testing

@testable import Monaco

struct SampleAPIChatTests {
    @Test func theChatSampleAnswersEveryRequestTheChatTabMakes() async throws {
        SampleAPIProtocol.install(SampleAPIScript(role: "member"))
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [SampleAPIProtocol.self]
        let session = URLSession(configuration: configuration)
        let cabal = "01920000-0000-7000-8000-000000000001"
        let message = "01920000-0000-7000-8000-000000000007"
        let requests: [(method: String, path: String)] = [
            ("GET", "/v1/cabals/\(cabal)/messages"),
            ("GET", "/v1/cabals/\(cabal)/messages/\(message)/thread"),
            ("POST", "/v1/cabals/\(cabal)/chat/seen"),
            ("GET", "/v1/cabals/\(cabal)/chat/seen?message_id=\(message)"),
            ("POST", "/v1/realtime/token"),
            ("GET", "/v1/me/blocks"),
        ]

        for (method, path) in requests {
            var request = try URLRequest(url: #require(URL(string: "https://sample.invalid\(path)")))
            request.httpMethod = method
            let (_, response) = try await session.data(for: request)
            let status = try #require(response as? HTTPURLResponse).statusCode
            #expect(status == 200, "\(method) \(path) answered \(status)")
        }

        #expect(SampleAPIProtocol.unhandledRequests.isEmpty)
    }
}
