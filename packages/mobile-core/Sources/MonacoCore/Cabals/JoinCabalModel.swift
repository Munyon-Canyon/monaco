import Foundation
import MonacoAPI
import Observation

#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct JoinedCabal: Equatable, Sendable {
    public let cabalID: String
    public let toast: String?

    public init(cabalID: String, toast: String?) {
        self.cabalID = cabalID
        self.toast = toast
    }
}

@Observable
@MainActor
public final class JoinCabalModel {
    public enum Lookup: Equatable, Sendable {
        case none
        case looking(InviteCode)
        case found(InviteCode, Components.Schemas.CabalPreview)
        case notFound(InviteCode)
        case failed(InviteCode, APIError)
    }

    public static let notFoundMessage = "No cabal with that invite code"
    public static let malformedMessage = "That doesn't look like an invite code. Check it and try again."
    public static let helper = "Paste the invite code your friend shared."

    public var code = ""
    public private(set) var lookup: Lookup = .none
    public private(set) var isSubmitting = false
    public private(set) var requestPending = false
    public private(set) var toast: CabalInviteToast?

    private let api: APIClient
    private let submission = IdempotentSubmission()
    private var toastSerial = 0

    public init(api: APIClient) {
        self.api = api
    }

    public var inviteCode: InviteCode? { InviteCode(code) }

    public var preview: Components.Schemas.CabalPreview? {
        guard case .found(let found, let preview) = lookup, found == inviteCode else { return nil }
        return preview
    }

    public var actionTitle: String {
        if isSubmitting { return "Sending…" }
        return requestPending ? "Request sent" : "Ask to join"
    }

    public var canSubmit: Bool {
        inviteCode != nil && !isSubmitting && !requestPending && !isNotFound
    }

    public var message: String? {
        guard let inviteCode else {
            let typed = code.trimmingCharacters(in: .whitespacesAndNewlines)
            return typed.count >= InviteCode.length ? Self.malformedMessage : nil
        }
        switch lookup {
        case .notFound(let code) where code == inviteCode: return Self.notFoundMessage
        case .failed(let code, let error) where code == inviteCode: return ToastCopy.message(for: error)
        default: return nil
        }
    }

    public var isError: Bool { message != nil }

    private var isNotFound: Bool {
        if case .notFound(let code) = lookup, code == inviteCode { return true }
        return false
    }

    public func lookUp() async {
        requestPending = false
        guard let code = inviteCode else {
            lookup = .none
            return
        }
        if case .found(code, _) = lookup { return }
        lookup = .looking(code)
        do {
            let preview = try await api.read { client in
                try await client.getCabalByCode(path: .init(code: code.value)).ok.body.json
            }
            guard inviteCode == code else { return }
            lookup = .found(code, preview)
        } catch {
            guard inviteCode == code else { return }
            let failure = APIError(error)
            lookup = APIClient.flow03Outcome(failure) == .cabalNotFound ? .notFound(code) : .failed(code, failure)
        }
    }

    public func submit() async -> JoinedCabal? {
        guard canSubmit else { return nil }
        isSubmitting = true
        defer { isSubmitting = false }
        if preview == nil {
            await lookUp()
        }
        guard let preview else { return nil }
        switch await api.enterCabal(preview.id, submission: submission) {
        case .requested: return JoinedCabal(cabalID: preview.id, toast: CabalEntry.requestedToast)
        case .alreadyMember: return JoinedCabal(cabalID: preview.id, toast: nil)
        case .requestPending:
            requestPending = true
            return nil
        case .refused(let error):
            if APIClient.flow03Outcome(error) == .cabalNotFound, let code = inviteCode {
                lookup = .notFound(code)
            } else {
                show(ToastCopy.message(for: error))
            }
            return nil
        }
    }

    private func show(_ message: String) {
        toastSerial += 1
        toast = CabalInviteToast(serial: toastSerial, message: message, isSuccess: false)
    }
}

#if DEBUG
extension JoinCabalModel {
    public static func preview() -> JoinCabalModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        return JoinCabalModel(
            api: APIClient(
                serverURL: serverURL, tokens: JoinCabalPreviewTokens(),
                transport: JoinCabalPreviewTransport()
            )
        )
    }
}

private struct JoinCabalPreviewTransport: ClientTransport {
    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let preview = Components.Schemas.CabalPreview.sample(joinMode: "request")
        let body: Data
        var response = HTTPResponse(status: .ok)
        switch operationID {
        case "getCabalByCode":
            body = try JSONEncoder().encode(preview)
        case "postCabalAccessRequest":
            response.status = .created
            body = Data(#"{"id":"\#(preview.id)","direction":"request","status":"pending"}"#.utf8)
        default:
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .iso8601
            body = try encoder.encode(Components.Schemas.Cabal.sample(role: "member"))
        }
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(body))
    }
}

private struct JoinCabalPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}
#endif
