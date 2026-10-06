import MonacoAPI

struct FeedMuteService: Sendable {
    let api: APIClient

    func mute(_ target: FeedMuteTarget) async throws {
        guard let type = Components.Schemas.FeedMuteRequest.TargetTypePayload(rawValue: target.kind.rawValue) else {
            return
        }
        let body = Components.Schemas.FeedMuteRequest(targetType: type, targetId: target.id)
        _ = try await api.submit(IdempotentSubmission(), payload: body, operation: "putMeFeedMutes") { client, key in
            try await client.putMeFeedMutes(headers: .init(idempotencyKey: key), body: .json(body)).noContent
        }
    }

    func unmute(_ target: FeedMuteTarget) async throws {
        guard
            let type = Operations.DeleteMeFeedMutesTargetTypeTargetID.Input.Path.TargetTypePayload(
                rawValue: target.kind.rawValue)
        else { return }
        let payload = "\(target.kind.rawValue)/\(target.id)"
        _ = try await api.submit(IdempotentSubmission(), payload: payload, operation: "deleteMeFeedMutes") {
            client, key in
            try await client.deleteMeFeedMutesTargetTypeTargetID(
                path: .init(targetType: type, targetId: target.id), headers: .init(idempotencyKey: key)
            ).noContent
        }
    }

    func list() async throws -> [Components.Schemas.FeedMute] {
        try await api.read { client in try await client.getMeFeedMutes().ok.body.json }
    }
}
