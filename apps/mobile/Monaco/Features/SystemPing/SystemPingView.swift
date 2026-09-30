#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

struct SystemPingView: View {
    @State private var model: SystemPingModel
    @State private var note = ""
    @State private var toast: MonacoToast?

    init(model: SystemPingModel) {
        _model = State(initialValue: model)
    }

    var body: some View {
        Form {
            Section("Note") {
                TextField("Note", text: $note)
                    .monacoFormTextField()
            }
            Section {
                Button("Send") {
                    let note = note
                    Task { await model.send(note: note) }
                }
                .monacoFormPrimaryAction()
                .disabled(sending || note.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
            Section("Ping") {
                status
            }
        }
        .monacoFormScreen()
        .navigationTitle("System ping")
        .navigationBarTitleDisplayMode(.inline)
        .task { await model.observe() }
        .onChange(of: model.state) { _, newState in
            guard case .failed(let error) = newState else { return }
            toast = MonacoToast(message: ToastCopy.message(for: error))
        }
        .monacoToast($toast)
    }

    private var sending: Bool {
        model.isSending
    }

    @ViewBuilder
    private var status: some View {
        switch model.state {
        case .idle:
            Text("Not sent")
                .foregroundStyle(MonacoTheme.secondaryText)
        case .loading:
            ProgressView()
                .tint(MonacoTheme.ink)
        case .loaded(let ping):
            Text(ping.echoed ? "Echoed" : "Waiting")
                .foregroundStyle(ping.echoed ? MonacoTheme.ink : MonacoTheme.secondaryText)
            Text(ping.note)
                .foregroundStyle(MonacoTheme.primaryText)
        case .failed(let error):
            Text(ToastCopy.message(for: error))
                .foregroundStyle(MonacoTheme.primaryText)
        }
    }
}

#Preview {
    let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
    NavigationStack {
        SystemPingView(
            model: SystemPingModel(
                api: APIClient(
                    serverURL: serverURL,
                    tokens: SystemPingPreviewTokens(),
                    transport: SystemPingPreviewTransport()
                ),
                hints: SystemPingPreviewHints()
            )
        )
    }
}

private struct SystemPingPreviewTransport: ClientTransport {
    func send(
        _ request: HTTPRequest,
        body _: HTTPBody?,
        baseURL _: URL,
        operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        let status: HTTPResponse.Status = request.method == .post ? .created : .ok
        var response = HTTPResponse(status: status)
        response.headerFields[.contentType] = "application/json"
        return (
            response,
            HTTPBody(#"{"id":"00000000-0000-4000-8000-000000000001","note":"hi","echoed":true}"#)
        )
    }
}

private struct SystemPingPreviewTokens: MonacoAPI.AccessTokenProvider {
    func accessToken() async throws -> String? { nil }
    func refreshedToken(replacing: String) async throws -> String? { nil }
}

private struct SystemPingPreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> {
        AsyncStream { continuation in
            continuation.finish()
        }
    }
}
#endif
