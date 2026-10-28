import Foundation
import MonacoAPI
import SwiftUI
import Testing
import UIKit

@testable import Monaco

private final class ToastClock {
    private var date = Date(timeIntervalSinceReferenceDate: 1_000)

    func now() -> Date { date }

    func advance(_ seconds: TimeInterval) { date = date.addingTimeInterval(seconds) }
}

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

    @Test func theSameErrorWithinTheBurstKeepsTheFirstToast() {
        let clock = ToastClock()
        let center = ToastCenter(now: clock.now)
        center.current = MonacoToast(message: "You're offline. Try again.")
        let first = center.current
        clock.advance(0.3)
        center.current = MonacoToast(message: "You're offline. Try again.")
        #expect(center.current?.id == first?.id)
    }

    @Test func theSameErrorAfterTheBurstShowsAgain() {
        let clock = ToastClock()
        let center = ToastCenter(now: clock.now)
        center.current = MonacoToast(message: "You're offline. Try again.")
        let first = center.current
        clock.advance(1.5)
        center.current = MonacoToast(message: "You're offline. Try again.")
        #expect(center.current?.message == "You're offline. Try again.")
        #expect(center.current?.id != first?.id)
    }

    @Test func aDifferentErrorWithinTheBurstShows() {
        let clock = ToastClock()
        let center = ToastCenter(now: clock.now)
        center.current = MonacoToast(message: "You're offline. Try again.")
        clock.advance(0.3)
        center.current = MonacoToast(message: "Couldn't load the pot.")
        #expect(center.current?.message == "Couldn't load the pot.")
    }

    @Test func aSuccessWithinTheBurstShows() {
        let clock = ToastClock()
        let center = ToastCenter(now: clock.now)
        center.current = MonacoToast(message: "You're offline. Try again.")
        clock.advance(0.3)
        center.show(success: "Funded this cabal.")
        #expect(center.current?.message == "Funded this cabal.")
        #expect(center.current?.isSuccess == true)
    }

    @Test func settingNilClearsTheToast() {
        let clock = ToastClock()
        let center = ToastCenter(now: clock.now)
        center.current = MonacoToast(message: "You're offline. Try again.")
        clock.advance(0.3)
        center.current = nil
        #expect(center.current == nil)
    }

    @Test func aBottomCTALeavesTheToastAlone() async throws {
        let center = ToastCenter()
        center.show(success: "Voted Yes. 6th of 8.")
        let shown = try #require(center.current)
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

        await settle()
        #expect(center.current?.id == shown.id)

        host.rootView = AnyView(EmptyView().environment(center))
        await settle()
        #expect(center.current?.id == shown.id)
    }

    private func settle() async {
        for _ in 0..<5 {
            await withCheckedContinuation { continuation in
                RunLoop.main.perform { continuation.resume() }
            }
        }
    }
}
