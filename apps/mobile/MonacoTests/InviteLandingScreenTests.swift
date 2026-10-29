import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct InviteLandingScreenTests {
    private static let bodyIDs = [
        "invite-landing", "invite-landing-cover", "invite-landing-identity", "invite-landing-inviter",
        "invite-landing-members", "invite-landing-stats", "invite-landing-share",
    ]

    private static func identifiers(of state: InviteLandingSampleState, until id: String) async throws -> Set<String> {
        try AccessibilityTree.setAutomation(enabled: true)
        defer { try? AccessibilityTree.setAutomation(enabled: false) }
        let window = try ProposalTestWindow.hosting(InviteLandingSampleHarness(state: state))
        defer { window.isHidden = true }
        await ProposalTestWindow.until { AccessibilityTree.identifiers(in: window).contains(id) }
        for _ in 0..<300 { await Task.yield() }
        return Set(AccessibilityTree.identifiers(in: window))
    }

    @Test(.timeLimit(.minutes(1))) func theSignedOutFrameDrawsEveryPartAndTheNote() async throws {
        let ids = try await Self.identifiers(of: .frame, until: "invite-landing-join")
        for id in Self.bodyIDs + ["invite-landing-proposal", "invite-landing-join", "invite-landing-note"] {
            #expect(ids.contains(id), "\(id) missing; identifiers: \(ids)")
        }
        #expect(!ids.contains("invite-landing-accept") && !ids.contains("invite-landing-decline"))
    }

    @Test(.timeLimit(.minutes(1))) func signedInDrawsTheJoinButtonWithoutTheNote() async throws {
        let ids = try await Self.identifiers(of: .signedIn, until: "invite-landing-join")
        for id in Self.bodyIDs + ["invite-landing-proposal", "invite-landing-join"] {
            #expect(ids.contains(id), "\(id) missing; identifiers: \(ids)")
        }
        #expect(!ids.contains("invite-landing-note"), "\(ids)")
    }

    @Test(.timeLimit(.minutes(1))) func noOpenProposalLeavesOutTheProposalCard() async throws {
        let ids = try await Self.identifiers(of: .noProposal, until: "invite-landing-join")
        #expect(ids.contains("invite-landing-stats") && ids.contains("invite-landing-join"), "\(ids)")
        #expect(!ids.contains("invite-landing-proposal"))
    }

    @Test(.timeLimit(.minutes(1))) func requestModeAndRequestSentKeepTheJoinButton() async throws {
        for state in [InviteLandingSampleState.request, .requestSent] {
            let ids = try await Self.identifiers(of: state, until: "invite-landing-join")
            #expect(ids.contains("invite-landing-join"), "\(state): \(ids)")
            #expect(!ids.contains("invite-landing-accept"), "\(state)")
        }
    }

    @Test(.timeLimit(.minutes(1))) func anExistingInvitationShowsAcceptAndDecline() async throws {
        let ids = try await Self.identifiers(of: .invited, until: "invite-landing-accept")
        #expect(ids.contains("invite-landing-accept") && ids.contains("invite-landing-decline"), "\(ids)")
        #expect(!ids.contains("invite-landing-join"))
    }

    @Test(.timeLimit(.minutes(1))) func nullMonthFactsStillDrawTheStatCard() async throws {
        let ids = try await Self.identifiers(of: .monthFactsNull, until: "invite-landing-stats")
        #expect(ids.contains("invite-landing-stats") && !ids.contains("invite-landing-proposal"), "\(ids)")
    }

    @Test(.timeLimit(.minutes(1))) func loadingDrawsNoDockAndFailedDrawsTheRetry() async throws {
        let loading = try await Self.identifiers(of: .loading, until: "invite-landing-loading")
        #expect(loading.contains("invite-landing-loading") && !loading.contains("invite-landing-join"), "\(loading)")
        let failed = try await Self.identifiers(of: .failed, until: "invite-landing-failed")
        #expect(failed.contains("invite-landing-failed") && !failed.contains("invite-landing-join"), "\(failed)")
    }
}
