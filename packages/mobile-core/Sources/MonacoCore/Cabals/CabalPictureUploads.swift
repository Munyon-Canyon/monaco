import Foundation
import MonacoAPI

public final class CabalPictureUploads: Sendable {
    private let api: APIClient
    private let upload = IdempotentSubmission()
    private let removal = IdempotentSubmission()

    public init(api: APIClient) {
        self.api = api
    }

    public func setPicture(cabalID: String, imageData: Data, mimeType: String) async throws -> String? {
        let filename = mimeType == "image/png" ? "cabal.png" : "cabal.jpg"
        let cabal = try await api.submit(upload, payload: imageData, operation: "putCabalPicture \(cabalID)") {
            client, key in
            try await client.putCabalPicture(
                path: .init(id: cabalID),
                headers: .init(idempotencyKey: key),
                body: .multipartForm([.picture(.init(payload: .init(body: HTTPBody(imageData)), filename: filename))])
            ).ok.body.json
        }
        return cabal.pictureUrl
    }

    public func removePicture(cabalID: String) async throws -> String? {
        let cabal = try await api.submit(removal, payload: cabalID, operation: "deleteCabalPicture") { client, key in
            try await client.deleteCabalPicture(path: .init(id: cabalID), headers: .init(idempotencyKey: key)).ok.body
                .json
        }
        return cabal.pictureUrl
    }
}
