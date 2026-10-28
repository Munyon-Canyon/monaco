#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import Synchronization

enum FundCabalSampleState: String, CaseIterable {
    case empty
    case typed
    case invalid
    case settled
    case paused
    case belowMinimum
    case unconfirmed
    case submitting
    case pending
    case failed
    case noCash
    case loading
    case unavailable

    static let launchArgument = "-MonacoFundCabalSample"

    static func matching(_ arguments: [String]) -> FundCabalSampleState? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return FundCabalSampleState(rawValue: arguments[flag + 1])
    }

    var amountText: String {
        switch self {
        case .empty, .paused, .noCash, .loading, .unavailable: ""
        case .typed, .settled, .unconfirmed, .submitting, .pending, .failed: "250"
        case .invalid: "650"
        case .belowMinimum: "0.5"
        }
    }

    var submits: Bool {
        switch self {
        case .settled, .unconfirmed, .submitting, .pending, .failed: true
        default: false
        }
    }

    var server: FundCabalSampleServer {
        switch self {
        case .paused: FundCabalSampleServer(preview: .paused)
        case .noCash: FundCabalSampleServer(preview: .noCash)
        case .loading: FundCabalSampleServer(preview: .hang)
        case .unavailable: FundCabalSampleServer(preview: .unavailable)
        case .unconfirmed: FundCabalSampleServer(fund: .dropped)
        case .submitting: FundCabalSampleServer(fund: .hang)
        case .pending: FundCabalSampleServer(transfer: .submitted)
        case .failed: FundCabalSampleServer(transfer: .failed)
        default: FundCabalSampleServer()
        }
    }
}

struct FundCabalSampleServer: ClientTransport {
    enum PreviewAnswer: Sendable {
        case live
        case paused
        case noCash
        case hang
        case unavailable
    }

    enum FundAnswer: Sendable {
        case accepted
        case dropped
        case hang
        case refused(status: Int, code: String, message: String)
    }

    enum TransferAnswer: Sendable {
        case settled
        case submitted
        case failed
    }

    static let cabal = Components.Schemas.CabalRef.sampleACCapital
    static let availableMicros: Int64 = 500_000_000

    var preview = PreviewAnswer.live
    var fund = FundAnswer.accepted
    var transfer = TransferAnswer.settled

    private let amount = Amount()

    private final class Amount: Sendable {
        let micros = Mutex("0")
    }

    func send(
        _ request: HTTPRequest, body: HTTPBody?, baseURL _: URL, operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        switch operationID {
        case "getFundPreview": return try await previewReply(request)
        case "fundCabal": return try await fundReply(body)
        case "getFundTransfer": return try Self.json(.ok, transferWire())
        default: return Self.problem(404, "not_found", "Not in the sample data.")
        }
    }

    private func previewReply(_ request: HTTPRequest) async throws -> (HTTPResponse, HTTPBody?) {
        let query = URLComponents(string: request.path ?? "")?.queryItems ?? []
        let micros = query.first { $0.name == "amount_micros" }?.value.flatMap { Int64($0) } ?? 0
        switch preview {
        case .hang:
            try await Task.sleep(for: .seconds(3600))
            throw CancellationError()
        case .unavailable:
            return Self.problem(503, "unavailable", "Fund can't be read right now.")
        case .paused:
            return try Self.json(.ok, Components.Schemas.FundPreview.samplePaused)
        case .noCash:
            var wire = Components.Schemas.FundPreview.sample
            wire.availableMicros = "0"
            return try Self.json(.ok, wire)
        case .live:
            return try Self.json(.ok, liveWire(micros: micros))
        }
    }

    private func liveWire(micros: Int64) -> Components.Schemas.FundPreview {
        guard micros > 0 else { return .sample }
        guard micros > Self.availableMicros else { return .sampleTyped }
        var wire = Components.Schemas.FundPreview.sampleOver
        wire.overMicros = String(micros - Self.availableMicros)
        return wire
    }

    private func fundReply(_ body: HTTPBody?) async throws -> (HTTPResponse, HTTPBody?) {
        if let body, let data = try? await Data(collecting: body, upTo: 1 << 16),
            let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
            let posted = object["amount_micros"] as? String
        {
            amount.micros.withLock { $0 = posted }
        }
        switch fund {
        case .accepted: return try Self.json(.accepted, Components.Schemas.FundAccepted.sample)
        case .dropped: throw URLError(.networkConnectionLost)
        case .hang:
            try await Task.sleep(for: .seconds(3600))
            throw CancellationError()
        case .refused(let status, let code, let message): return Self.problem(status, code, message)
        }
    }

    private func transferWire() -> Components.Schemas.FundTransfer {
        let micros = amount.micros.withLock { $0 }
        switch transfer {
        case .settled:
            return Components.Schemas.FundTransfer(
                status: .settled, amountMicros: micros, shareUnits: micros, sliceBps: 3600, failCode: nil)
        case .submitted:
            return Components.Schemas.FundTransfer(
                status: .submitted, amountMicros: micros, shareUnits: nil, sliceBps: nil, failCode: nil)
        case .failed:
            return Components.Schemas.FundTransfer(
                status: .failed, amountMicros: micros, shareUnits: nil, sliceBps: nil, failCode: "fund_not_sent")
        }
    }

    private static func json(
        _ status: HTTPResponse.Status, _ value: some Encodable
    ) throws -> (HTTPResponse, HTTPBody?) {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: status)
        response.headerFields[.contentType] = "application/json"
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

struct FundCabalSampleHarness: View {
    let amountText: String
    let submits: Bool

    @State private var model: FundCabalModel

    init(state: FundCabalSampleState) {
        self.init(amountText: state.amountText, submits: state.submits, server: state.server)
    }

    init(amountText: String, submits: Bool, server: FundCabalSampleServer) {
        self.amountText = amountText
        self.submits = submits
        let api = APIClient(
            serverURL: URL(filePath: "/sample"),
            tokens: SessionTokens(privyToken: { "sample-token" }, refresh: { _ in nil }), transport: server)
        let hints = SampleSilentHints()
        let settlements = SettlementWatcher(api: api, hints: hints, journal: PendingIntentJournal())
        settlements.bind(accountID: "sample")
        _model = State(
            initialValue: FundCabalModel(
                api: api, hints: hints, cabalID: FundCabalSampleServer.cabal.id,
                cabalName: FundCabalSampleServer.cabal.name, submission: IdempotentSubmission(),
                settlements: settlements, clock: ContinuousClock()))
    }

    var body: some View {
        NavigationStack {
            FundCabalKeypadView(
                model: model, sources: Self.sources, onClose: {}, onPick: { _ in }
            )
            .toolbar(.hidden, for: .navigationBar)
        }
        .tint(MonacoTheme.ink)
        .task { await run() }
    }

    static let sources = [
        FundCabalSampleServer.cabal, Components.Schemas.CabalRef.sampleTechTitans,
    ].enumerated().map { index, cabal in
        FundSourceRow(id: cabal.id, name: cabal.name, ownership: index == 0 ? "35% yours" : "12% yours")
    }

    private func run() async {
        await model.load()
        guard !amountText.isEmpty else { return }
        model.amountText = amountText
        model.amountChanged()
        try? await Task.sleep(for: FundCabalModel.debounce + .milliseconds(300))
        if submits { await model.submit() }
    }
}

final class FundCabalSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let state = FundCabalSampleState.matching(arguments) else { return nil }
        return AnyView(FundCabalSampleHarness(state: state))
    }
}
#endif
