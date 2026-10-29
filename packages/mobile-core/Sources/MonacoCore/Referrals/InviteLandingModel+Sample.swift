#if DEBUG
import Foundation
import MonacoAPI
import MonacoReferrals
import OpenAPIRuntime

extension InviteLandingModel {
    public static func sample(
        code: String, cabalID: String, transport: some ClientTransport, hints: (any HintSource)?, viewerID: String?,
        pendingStore: PendingLinkTargetStore
    ) -> InviteLandingModel {
        let api = APIClient(
            serverURL: URL(filePath: "/sample"),
            tokens: FlowScenarioTokens(),
            transport: transport)
        return InviteLandingModel(
            code: code, cabalID: cabalID, api: api, hints: hints, viewerID: viewerID, pendingStore: pendingStore)
    }
}
#endif
