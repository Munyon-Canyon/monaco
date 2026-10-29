#if DEBUG
import MonacoAPI
import MonacoCore
import MonacoReferrals
import SwiftUI

nonisolated enum InviteLandingSampleState: String, CaseIterable {
    case frame
    case signedIn
    case noProposal
    case request
    case invited
    case monthFactsNull
    case requestSent
    case loading
    case failed

    static let launchArgument = "-MonacoInviteLandingSample"

    static func matching(_ arguments: [String]) -> InviteLandingSampleState? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return InviteLandingSampleState(rawValue: arguments[flag + 1])
    }

    var signedIn: Bool {
        switch self {
        case .signedIn, .request, .invited, .requestSent: true
        default: false
        }
    }

    var preview: Components.Schemas.ReferralCabal {
        var preview = Components.Schemas.ReferralCabal.sample
        preview.openProposal?.expiresAt = Date(timeIntervalSinceNow: 4 * 3600 + 60)
        switch self {
        case .frame, .signedIn, .invited: preview.cabal?.joinMode = .open
        case .noProposal: preview = .sampleNoOpenProposal
        case .monthFactsNull: preview = .sampleMonthFactsNull
        default: break
        }
        return preview
    }

    var cabal: Components.Schemas.Cabal {
        let preview = preview.cabal
        var cabal = Components.Schemas.Cabal.sample(role: nil)
        cabal.id = preview?.id ?? cabal.id
        cabal.name = preview?.name ?? cabal.name
        cabal.rules.joinMode = preview?.joinMode.rawValue ?? cabal.rules.joinMode
        switch self {
        case .invited: cabal.myAccessRequest = .init(id: "sample-invite", direction: "invite", status: "pending")
        case .requestSent: cabal.myAccessRequest = .init(id: "sample-request", direction: "request", status: "pending")
        default: break
        }
        return cabal
    }
}

struct InviteLandingSampleServer: ClientTransport {
    let state: InviteLandingSampleState

    func send(
        _ request: HTTPRequest, body: HTTPBody?, baseURL _: URL, operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        switch (operationID, state) {
        case ("getReferralCabal", .loading):
            try await Task.sleep(for: .seconds(3600))
            throw CancellationError()
        case ("getReferralCabal", .failed):
            return Self.problem(503, "unavailable", "The invite can't be read right now.")
        case ("getReferralCabal", _): return try Self.json(.ok, state.preview)
        case ("getCabal", _): return try Self.json(.ok, state.cabal)
        case ("getCabalAccessRequests", _): return try Self.json(.ok, [Components.Schemas.CabalAccessRequest]())
        default: return Self.problem(404, "not_found", "Not in the sample data.")
        }
    }

    private static func json(
        _ status: HTTPResponse.Status, _ value: some Encodable
    ) throws -> (HTTPResponse, HTTPBody?) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: status)
        response.headerFields[.contentType] = "application/json"
        response.headerFields[.cacheControl] = "no-cache"
        return (response, HTTPBody(try encoder.encode(value)))
    }

    private static func problem(_ status: Int, _ code: String, _ message: String) -> (HTTPResponse, HTTPBody?) {
        var response = HTTPResponse(status: .init(code: status))
        response.headerFields[.contentType] = "application/problem+json"
        let body =
            #"{"type":"about:blank","title":"Error","status":\#(status),"code":"\#(code)","#
            + #""message":"\#(message)","trace_id":"00000000000000000000000000000000","retryable":false}"#
        return (response, HTTPBody(body))
    }
}

struct InviteLandingSampleHarness: View {
    @State private var model: InviteLandingModel
    @State private var toasts = ToastCenter()

    init(state: InviteLandingSampleState) {
        _model = State(
            initialValue: .sample(
                code: "k7m4qx2p", cabalID: Components.Schemas.ReferralCabal.sample.cabal?.id ?? "",
                transport: InviteLandingSampleServer(state: state),
                hints: state.signedIn ? SampleSilentHints() : nil, viewerID: state.signedIn ? "sample-viewer" : nil,
                pendingStore: PendingLinkTargetStore(
                    store: UserDefaults(suiteName: "invite-landing-sample") ?? .standard)))
    }

    var body: some View {
        NavigationStack {
            InviteLandingScreen(
                model: model, shareURL: URL(string: "https://monacolabs.xyz/r/k7m4qx2p") ?? URL(filePath: "/"),
                onRedirect: { _ in })
        }
        .environment(toasts)
        .monacoToastCenter(toasts)
        .tint(MonacoTheme.ink)
        .task { await model.run() }
    }
}

final class InviteLandingSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let state = InviteLandingSampleState.matching(arguments) else { return nil }
        return AnyView(InviteLandingSampleHarness(state: state))
    }
}
#endif
