import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct WithdrawEstimateTests {
    private let evmAddress = "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
    private let solanaAddress = "9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"
    private let expiresAt = Date(timeIntervalSince1970: Date().timeIntervalSince1970.rounded(.down) + 300)

    private var previewFixture: String {
        #"{"receive_micros":"9992500","expires_at":"\#(ISO8601DateFormatter().string(from: expiresAt))"}"#
    }

    private var beforeExpiry: Date { expiresAt.addingTimeInterval(-1) }

    private var balance: AccountBalance {
        AccountBalance(
            availableMicros: 20_000_000, onChainMicros: 20_000_000, inFlightMicros: 0, depositAddress: "dep",
            asOf: Date())
    }

    private func form(
        _ network: WithdrawNetwork, address: String, amount: String = "10", estimate: Int64? = nil
    ) -> WithdrawForm {
        WithdrawForm(
            amountText: amount, destinationAddress: address, network: network, estimatedReceiveMicros: estimate,
            balance: balance)
    }

    private func source(_ transport: StubTransport) -> WithdrawSource {
        WithdrawSource(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport))
    }

    @Test func onlyAnEVMWithdrawalReadyToContinueAsksForAPreview() {
        #expect(
            form(.base, address: evmAddress).previewRequest
                == WithdrawPreviewRequest(micros: 10_000_000, toAddress: evmAddress, network: .base))
        #expect(form(.solana, address: solanaAddress).previewRequest == nil)
        #expect(form(.arbitrum, address: "").previewRequest == nil)
        #expect(form(.arbitrum, address: solanaAddress).previewRequest == nil)
        #expect(form(.ethereum, address: evmAddress, amount: "").previewRequest == nil)
        #expect(form(.ethereum, address: evmAddress, amount: "50").previewRequest == nil)
    }

    @Test func anEVMWithdrawalShowsTheEstimateFromThePreviewRoute() async throws {
        let request = try #require(form(.base, address: evmAddress).previewRequest)
        let transport = StubTransport(.json(.ok, previewFixture))

        let estimate = await WithdrawEstimate.fetch(request, from: source(transport), after: .zero)

        #expect(await transport.sent.map(\.path) == ["/v1/me/withdrawals/preview"])
        let shown = form(.base, address: evmAddress, estimate: estimate?.receiveMicros(for: request, at: beforeExpiry))
        #expect(shown.receiveLine == "You'll receive ~$9.99")
    }

    @Test func anEstimateForAnotherAmountOrNetworkIsNotShown() async throws {
        let request = try #require(form(.base, address: evmAddress).previewRequest)
        let transport = StubTransport(.json(.ok, previewFixture))
        let estimate = try #require(await WithdrawEstimate.fetch(request, from: source(transport), after: .zero))

        let otherAmount = form(.base, address: evmAddress, amount: "11").previewRequest
        let otherNetwork = form(.arbitrum, address: evmAddress).previewRequest
        #expect(estimate.receiveMicros(for: otherAmount, at: beforeExpiry) == nil)
        #expect(estimate.receiveMicros(for: otherNetwork, at: beforeExpiry) == nil)
        #expect(estimate.receiveMicros(for: nil, at: beforeExpiry) == nil)
    }

    @Test func aPreviewErrorHidesTheLineAndKeepsContinue() async throws {
        let request = try #require(form(.base, address: evmAddress).previewRequest)
        let transport = try StubTransport.problem(
            Components.Schemas.Problem(
                _type: .about_colon_blank, title: "Unavailable", status: 503, code: .privyUnavailable,
                message: "Try again.", traceId: "4bf92f3577b34da6a3ce929d0e0e4736", retryable: true))

        let estimate = await WithdrawEstimate.fetch(request, from: source(transport), after: .zero)

        #expect(estimate == nil)
        let shown = form(.base, address: evmAddress, estimate: estimate?.receiveMicros(for: request, at: beforeExpiry))
        #expect(shown.receiveLine == nil)
        #expect(shown.canContinue)
    }

    @Test func anEstimateIsShownUntilItsQuoteExpires() async throws {
        let request = try #require(form(.base, address: evmAddress).previewRequest)
        let transport = StubTransport(.json(.ok, previewFixture))
        let estimate = try #require(await WithdrawEstimate.fetch(request, from: source(transport), after: .zero))

        #expect(estimate.expiresAt == expiresAt)
        #expect(estimate.receiveMicros(for: request, at: beforeExpiry) == 9_992_500)
        #expect(estimate.receiveMicros(for: request, at: expiresAt) == nil)
        #expect(estimate.receiveMicros(for: request, at: expiresAt.addingTimeInterval(60)) == nil)
    }

    @Test func anExpiredEstimateIsNotShownAgainAfterEditingBackToItsRequest() throws {
        let a = try #require(form(.base, address: evmAddress).previewRequest)
        let b = try #require(form(.base, address: evmAddress, amount: "11").previewRequest)
        let estimate = WithdrawEstimate(request: a, receiveMicros: 9_992_500, expiresAt: expiresAt)

        #expect(estimate.receiveMicros(for: a, at: beforeExpiry) == 9_992_500)
        #expect(estimate.receiveMicros(for: b, at: beforeExpiry) == nil)
        #expect(estimate.receiveMicros(for: a, at: expiresAt.addingTimeInterval(1)) == nil)
    }

    @Test func aSolanaEstimateWithNoExpiryStaysShown() throws {
        let request = try #require(form(.base, address: evmAddress).previewRequest)
        let estimate = WithdrawEstimate(request: request, receiveMicros: 9_992_500, expiresAt: nil)

        #expect(estimate.receiveMicros(for: request, at: expiresAt.addingTimeInterval(3600)) == 9_992_500)
    }

    @Test func theRefetchWaitsForTheTimeLeftAndNotAtAllOncePast() {
        let ninetyBefore = expiresAt.addingTimeInterval(-90)
        #expect(WithdrawEstimate.refetchDelay(until: expiresAt, now: ninetyBefore) == .seconds(90))
        #expect(WithdrawEstimate.refetchDelay(until: expiresAt, now: expiresAt) == .zero)
        #expect(WithdrawEstimate.refetchDelay(until: expiresAt, now: expiresAt.addingTimeInterval(30)) == .zero)
    }

    @Test func aQuoteAlwaysAlreadyExpiredBacksOffInsteadOfSpinning() async throws {
        let request = try #require(form(.base, address: evmAddress).previewRequest)
        let start = Date()
        let past = ISO8601DateFormatter().string(from: start.addingTimeInterval(-60))
        let transport = StubTransport(.json(.ok, #"{"receive_micros":"9992500","expires_at":"\#(past)"}"#))
        var clock = start
        var waits: [Duration] = []
        var shown: [Int64?] = []

        await WithdrawEstimate.follow(
            request, from: source(transport), debounce: .zero, now: { clock },
            sleep: { wait in
                waits.append(wait)
                clock += Double(wait.components.seconds)
                if clock.timeIntervalSince(start) > 60 { throw CancellationError() }
            },
            update: { shown.append($0?.receiveMicros) })

        #expect(waits == [2, 4, 8, 15, 15, 15, 15].map { Duration.seconds($0) })
        #expect(await transport.sent.count == 7)
        #expect(shown.count == 13)
    }

    @Test func aLiveQuoteIsRefetchedWhenItExpires() async throws {
        let request = try #require(form(.base, address: evmAddress).previewRequest)
        let transport = StubTransport(.json(.ok, previewFixture))
        let now = expiresAt.addingTimeInterval(-300)
        var waits: [Duration] = []

        await WithdrawEstimate.follow(
            request, from: source(transport), debounce: .zero, now: { now },
            sleep: { wait in
                waits.append(wait)
                throw CancellationError()
            },
            update: { _ in })

        #expect(waits == [.seconds(300)])
        #expect(await transport.sent.count == 1)
    }
}
