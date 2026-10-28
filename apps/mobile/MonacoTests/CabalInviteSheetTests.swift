import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct CabalInviteSheetTests {
    private static let codePath = "/v1/me/referral-code"
    private static let code =
        #"{"code":"k7m4qx2p","link":"https://monacolabs.xyz/r/k7m4qx2p","handle_link":null,"handle_unlocked":false}"#

    private enum Preview {
        case frame, noProposal, hang, fail
    }

    @Test(.timeLimit(.minutes(1)))
    func theFrameShowsTheCardTheProposalAndTheAdminGear() async throws {
        let identifiers = try await identifiers(role: "creator", preview: .frame, ready: "cabal-invite-proposal")

        for id in [
            "cabal-invite-title", "cabal-invite-link", "cabal-invite-cover", "cabal-invite-facts",
            "cabal-invite-inviter", "cabal-invite-proposal", "cabal-invite-settings", "cabal-invite-qr-toggle",
            "share-card-share", "share-card-copy",
        ] {
            #expect(identifiers.contains(id), "\(id) missing; identifiers: \(identifiers)")
        }
        #expect(!identifiers.contains("cabal-invite-gain-row"), "identifiers: \(identifiers)")
        #expect(!identifiers.contains("cabal-invite-qr"), "the QR starts on the back: \(identifiers)")
    }

    @Test(.timeLimit(.minutes(1)))
    func noOpenProposalLeavesNoProposalRow() async throws {
        let identifiers = try await identifiers(role: "creator", preview: .noProposal, ready: "cabal-invite-facts")

        #expect(identifiers.contains("cabal-invite-inviter"), "identifiers: \(identifiers)")
        #expect(!identifiers.contains("cabal-invite-proposal"), "identifiers: \(identifiers)")
    }

    @Test(
        .timeLimit(.minutes(1)),
        arguments: [
            (role: "creator", showsGear: true), (role: "admin", showsGear: true), (role: "member", showsGear: false),
        ])
    func onlyAdminsSeeTheGear(role: String, showsGear: Bool) async throws {
        let identifiers = try await identifiers(role: role, preview: .frame, ready: "share-card-copy")

        #expect(identifiers.contains("cabal-invite-settings") == showsGear, "identifiers: \(identifiers)")
        #expect(identifiers.contains("share-card-share"), "every member can share: \(identifiers)")
    }

    @Test(.timeLimit(.minutes(1)))
    func whileTheCardLoadsTheSkeletonShowsAndNothingCanBeShared() async throws {
        let identifiers = try await identifiers(role: "member", preview: .hang, ready: "cabal-invite-loading")

        #expect(!identifiers.contains("cabal-invite-link"), "identifiers: \(identifiers)")
        #expect(identifiers.contains("share-card-share"), "identifiers: \(identifiers)")
    }

    @Test(.timeLimit(.minutes(1)))
    func aFailedPreviewOffersARetry() async throws {
        let identifiers = try await identifiers(role: "member", preview: .fail, ready: "cabal-invite-failed")

        #expect(identifiers.contains("cabal-invite-retry"), "identifiers: \(identifiers)")
    }

    @Test(arguments: [
        (mode: CabalInviteMode.invited, id: "cabal-invite-view"),
        (mode: CabalInviteMode.requested, id: "cabal-invite-view"),
        (mode: CabalInviteMode.unavailable, id: "cabal-invite-unavailable"),
    ])
    func theInviteeStatesKeepTheirIdentifiers(mode: CabalInviteMode, id: String) async throws {
        let transport = StubTransport(routes: [:])
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(
            hosting: CabalInviteSheet(cabal: .sample(role: nil), mode: mode), over: transport)
        defer { TestWindow.close(window) }

        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains(id) }

        let identifiers = AccessibilityTree.identifiers(in: window)
        #expect(identifiers.contains(id), "identifiers: \(identifiers)")
    }

    @Test func theQRNamesTheInviteLinkNotADepositAddress() throws {
        let label = CabalInviteQR.accessibilityLabelText(cabalName: "AC Capital")

        #expect(label == "QR code of the invite link to AC Capital")
        #expect(label != DepositQRCode.accessibilityLabelText)
        #expect(CabalInviteQR.caption(cabalName: "AC Capital") == "Scan to join AC Capital")
    }

    @Test(.timeLimit(.minutes(1)))
    func theQRTileCarriesTheLinkAndOnlyTheLinkLabel() async throws {
        let link = try #require(URL(string: "https://monacolabs.xyz/r/kaicenat?c=01890a5d-ac96-774b-bcce-b302099a8058"))
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try ProposalTestWindow.hosting(CabalInviteQR(link: link, cabalName: "AC Capital"))
        defer { TestWindow.close(window) }

        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains("cabal-invite-qr") }

        let labels = AccessibilityTree.labels(in: window)
        #expect(labels.contains("QR code of the invite link to AC Capital"), "labels: \(labels)")
        #expect(!labels.contains(DepositQRCode.accessibilityLabelText), "labels: \(labels)")
        #expect(QRImage.cgImage(for: link.absoluteString) != nil)
    }

    @Test func theSharedPreviewImageIsTheCard() throws {
        let preview = try #require(CabalInviteCopy.preview(.sample, now: Date(timeIntervalSince1970: 1_793_185_000)))

        let image = try #require(CabalInviteSheet.cardImage(preview))

        #expect(image.scale == CabalInviteMetrics.shareScale)
        #expect(image.size.width == CabalInviteMetrics.shareWidth)
        #expect(image.size.height > CabalInviteMetrics.coverHeight)
    }

    private func identifiers(role: String, preview: Preview, ready: String) async throws -> [String] {
        let transport = try Self.transport(preview)
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try Self.window(
            hosting: CabalInviteSheet(cabal: .sample(role: role)), over: transport)
        defer { TestWindow.close(window) }

        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains(ready) }
        for _ in 0..<200 { await Task.yield() }
        return AccessibilityTree.identifiers(in: window)
    }

    private static func transport(_ preview: Preview) throws -> StubTransport {
        let id = Components.Schemas.Cabal.sample(role: nil).id
        var source = Components.Schemas.ReferralCabal.sample
        source.cabal?.id = id
        let path = "/v1/referrals/k7m4qx2p/cabals/\(id)"
        let reply: StubTransport.Reply
        switch preview {
        case .frame: reply = try Self.previewReply(source)
        case .noProposal:
            source.openProposal = nil
            reply = try Self.previewReply(source)
        case .hang: reply = .hang
        case .fail: reply = .failure(URLError(.notConnectedToInternet))
        }
        return StubTransport(routes: [
            codePath: Array(repeating: .json(.ok, code), count: 4),
            path: Array(repeating: reply, count: 4),
        ])
    }

    private static func previewReply(_ source: Components.Schemas.ReferralCabal) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        var response = HTTPResponse(status: .ok)
        response.headerFields[.contentType] = "application/json"
        response.headerFields[.cacheControl] = "no-cache"
        return .response(response, try encoder.encode(source))
    }

    private static func window(hosting view: some View, over transport: StubTransport) throws -> UIWindow {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        let environment = AppEnvironment(
            auth: auth, api: api, hints: FakeHintSource(), isAuthenticated: { true }, endAuthSession: {})
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView:
                view
                .environment(environment)
                .environment(ToastCenter())
        )
        window.makeKeyAndVisible()
        return window
    }
}
