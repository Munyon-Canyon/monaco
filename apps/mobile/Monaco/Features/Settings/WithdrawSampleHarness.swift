#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI
import Synchronization

enum WithdrawSampleState: String, CaseIterable {
    case empty
    case typed
    case invalid
    case settled
    case needsAddress
    case invalidAddress
    case evmFee
    case feeUnknown
    case feeFailed
    case submitting

    static let launchArgument = "-MonacoWithdrawSample"
    static let solanaAddress = "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"
    static let evmAddress = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"

    static func matching(_ arguments: [String]) -> WithdrawSampleState? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return WithdrawSampleState(rawValue: arguments[flag + 1])
    }

    var amountText: String {
        switch self {
        case .empty, .feeUnknown, .feeFailed: ""
        case .typed, .settled, .needsAddress, .invalidAddress, .submitting: "120"
        case .invalid: "650"
        case .evmFee: "10"
        }
    }

    var address: String {
        switch self {
        case .needsAddress: ""
        case .invalidAddress: "not-an-address"
        case .evmFee, .feeUnknown: Self.evmAddress
        default: Self.solanaAddress
        }
    }

    var network: WithdrawNetwork {
        switch self {
        case .evmFee, .feeUnknown: .base
        default: .solana
        }
    }

    var submits: Bool {
        switch self {
        case .settled, .submitting: true
        default: false
        }
    }

    var server: WithdrawSampleServer {
        switch self {
        case .feeUnknown: WithdrawSampleServer(feeKnown: false)
        case .feeFailed: WithdrawSampleServer(previewFails: true)
        case .submitting: WithdrawSampleServer(withdraw: .hang)
        default: WithdrawSampleServer()
        }
    }
}

struct WithdrawSampleServer: ClientTransport {
    enum Answer: Sendable {
        case accepted
        case dropped
        case hang
        case refused(status: Int, code: String, message: String)
    }

    static let availableMicros: Int64 = 500_000_000
    static let solanaFeeMicros: Int64 = 500_000
    static let evmFeeMicros: Int64 = 7_500

    var feeKnown = true
    var previewFails = false
    var withdraw = Answer.accepted

    private let sent = Sent()

    private final class Sent: Sendable {
        let micros = Mutex<Int64>(0)
    }

    func send(
        _ request: HTTPRequest, body: HTTPBody?, baseURL _: URL, operationID: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        switch operationID {
        case "previewWithdrawal": return try await previewReply(body)
        case "authorizeWithdrawal": return try await authorizeReply()
        case "withdraw": return try await withdrawReply(body)
        case "getMyBalance": return try Self.json(.ok, balance())
        case "getMyWithdrawal": return try Self.json(.ok, withdrawal())
        default: return Self.problem(404, "not_found", "Not in the sample data.")
        }
    }

    private func previewReply(_ body: HTTPBody?) async throws -> (HTTPResponse, HTTPBody?) {
        if previewFails { return Self.problem(503, "unavailable", "The fee can't be read right now.") }
        let fields = await Self.fields(body)
        let micros = fields["amount_micros"].flatMap { Int64($0) } ?? 0
        guard fields["destination_kind"] == "evm" else { return try Self.json(.ok, solana(micros: micros)) }
        guard feeKnown else { return try Self.json(.ok, Components.Schemas.WithdrawalPreview.sampleEVMFeeUnknown) }
        var wire = Components.Schemas.WithdrawalPreview.sampleEVM
        if micros > 0 { wire.receiveMicros = String(max(micros - Self.evmFeeMicros, 0)) }
        return try Self.json(.ok, wire)
    }

    private func solana(micros: Int64) -> Components.Schemas.WithdrawalPreview {
        let most = Self.availableMicros - Self.solanaFeeMicros
        guard micros > 0 else { return .sampleEmpty }
        guard micros > most else {
            var wire = Components.Schemas.WithdrawalPreview.sampleTyped
            wire.receiveMicros = String(micros)
            return wire
        }
        var wire = Components.Schemas.WithdrawalPreview.sampleOver
        wire.overMicros = String(micros - most)
        return wire
    }

    private func authorizeReply() async throws -> (HTTPResponse, HTTPBody?) {
        if case .refused(let status, let code, let message) = withdraw { return Self.problem(status, code, message) }
        if case .dropped = withdraw { throw URLError(.networkConnectionLost) }
        let granted =
            #"{"authorization_id":"01890a5d-ac96-774b-bcce-b302099a8059","expires_at":"2099-01-01T00:00:00Z"}"#
        var response = HTTPResponse(status: .created)
        response.headerFields[.contentType] = "application/json"
        return (response, HTTPBody(granted))
    }

    private func withdrawReply(_ body: HTTPBody?) async throws -> (HTTPResponse, HTTPBody?) {
        let fields = await Self.fields(body)
        if let posted = fields["amount_micros"].flatMap({ Int64($0) }) { sent.micros.withLock { $0 = posted } }
        switch withdraw {
        case .accepted: return try Self.json(.accepted, Components.Schemas.WithdrawAccepted.sample)
        case .dropped: throw URLError(.networkConnectionLost)
        case .hang:
            try await Task.sleep(for: .seconds(3600))
            throw CancellationError()
        case .refused(let status, let code, let message): return Self.problem(status, code, message)
        }
    }

    private func balance() -> Components.Schemas.Balance {
        let spent = sent.micros.withLock { $0 }
        let paid = spent > 0 ? spent + Self.solanaFeeMicros : 0
        var wire = Components.Schemas.Balance.sample
        let available = Self.availableMicros - paid
        wire.availableMicros = String(available)
        wire.inFlightMicros = "0"
        wire.onChainMicros = String(available)
        return wire
    }

    private func withdrawal() -> Components.Schemas.Withdrawal {
        var wire = Components.Schemas.Withdrawal.sample
        wire.amountMicros = String(sent.micros.withLock { $0 })
        return wire
    }

    private static func fields(_ body: HTTPBody?) async -> [String: String] {
        guard let body, let data = try? await Data(collecting: body, upTo: 1 << 16),
            let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
        else { return [:] }
        return object.compactMapValues { $0 as? String }
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

struct WithdrawSampleHarness: View {
    let amountText: String
    let address: String
    let network: WithdrawNetwork
    let submits: Bool

    @State private var model: WithdrawModel

    init(state: WithdrawSampleState) {
        self.init(
            amountText: state.amountText, address: state.address, network: state.network, submits: state.submits,
            server: state.server)
    }

    init(amountText: String, address: String, network: WithdrawNetwork, submits: Bool, server: WithdrawSampleServer) {
        self.amountText = amountText
        self.address = address
        self.network = network
        self.submits = submits
        _model = State(initialValue: .sample(transport: server, ownDepositAddress: nil, clock: ContinuousClock()))
    }

    var body: some View {
        NavigationStack {
            WithdrawKeypadView(model: model, onClose: {})
                .toolbar(.hidden, for: .navigationBar)
        }
        .tint(MonacoTheme.ink)
        .task { await run() }
    }

    private func run() async {
        model.setNetwork(network)
        model.setAddress(address)
        model.setAmountText(amountText)
        model.load()
        guard submits else { return }
        try? await Task.sleep(for: .milliseconds(600))
        await model.submit()
    }
}

final class WithdrawSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let state = WithdrawSampleState.matching(arguments) else { return nil }
        return AnyView(WithdrawSampleHarness(state: state))
    }
}
#endif
