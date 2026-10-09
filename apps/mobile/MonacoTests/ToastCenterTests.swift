import Foundation
import MonacoAPI
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct ToastCenterTests {
    @Test func transportShowsOfflineCopy() {
        let center = ToastCenter()
        center.show(APIError.transport(URLError(.notConnectedToInternet)))
        #expect(center.current?.message == "You're offline. Try again.")
        #expect(center.current?.isSuccess == false)
    }

    @Test func problemShowsTheServerMessage() {
        let problem = ProblemError(
            status: 404,
            code: .known(.notFound),
            message: "No such cabal.",
            traceID: "4bf92f3577b34da6a3ce929d0e0e4736",
            retryable: false
        )
        let center = ToastCenter()
        center.show(.problem(problem))
        #expect(center.current?.message == "No such cabal.")
    }

    @Test func successMarksTheToast() {
        let center = ToastCenter()
        center.show(success: "Funded this cabal.")
        #expect(center.current?.message == "Funded this cabal.")
        #expect(center.current?.isSuccess == true)
    }

    @Test func theDefaultPlacementLiftsOnlyWhileABottomCTAIsOnScreen() {
        let center = ToastCenter()
        #expect(center.placement(requested: .screenBottom) == .screenBottom)
        center.bottomCTAs = 1
        #expect(center.placement(requested: .screenBottom) == .aboveBottomCTA)
        center.bottomCTAs = 0
        #expect(center.placement(requested: .screenBottom) == .screenBottom)
    }

    @Test func aPlacementTheCallerChoseWinsOverABottomCTA() {
        let center = ToastCenter()
        center.bottomCTAs = 1
        #expect(center.placement(requested: .custom(72)) == .custom(72))
        #expect(center.placement(requested: .aboveBottomCTA) == .aboveBottomCTA)
    }

    @Test func aMountedBottomCTACountsUntilItDisappears() async throws {
        let center = ToastCenter()
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene"
        )
        let window = UIWindow(windowScene: scene)
        window.frame = CGRect(x: 0, y: 0, width: 390, height: 844)
        let host = UIHostingController(
            rootView: AnyView(BottomCTA { Text("Send") }.environment(center)))
        window.rootViewController = host
        window.makeKeyAndVisible()
        defer { window.isHidden = true }

        try await settle { center.bottomCTAs == 1 }
        #expect(center.placement(requested: .screenBottom) == .aboveBottomCTA)

        host.rootView = AnyView(EmptyView().environment(center))
        try await settle { center.bottomCTAs == 0 }
        #expect(center.placement(requested: .screenBottom) == .screenBottom)
    }

    private func settle(_ done: () -> Bool) async throws {
        let deadline = ContinuousClock.now + .seconds(5)
        while !done() {
            try #require(ContinuousClock.now < deadline, "the bar's appear and disappear never reached the center")
            await withCheckedContinuation { continuation in
                RunLoop.main.perform { continuation.resume() }
            }
        }
    }
}
