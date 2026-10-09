import Foundation
import MonacoAPI
import Testing

@testable import Monaco

@MainActor
struct RequestCancellationTests {
    @Test func aCancelledAPIErrorIsARequestCancellation() {
        #expect(APIError.cancelled.isRequestCancellation)
    }

    @Test func otherAPIErrorsAreNotRequestCancellations() {
        #expect(!APIError.decoding("x").isRequestCancellation)
        #expect(!APIError.transport(URLError(.notConnectedToInternet)).isRequestCancellation)
    }

    @Test func aCancelledErrorShowsNoToast() {
        let center = ToastCenter()
        center.show(.cancelled)
        #expect(center.current == nil)
    }

    @Test func aCancelledErrorLeavesTheToastAlreadyOnScreen() {
        let center = ToastCenter()
        center.show(success: "Funded this cabal.")
        center.show(.cancelled)
        #expect(center.current?.message == "Funded this cabal.")
    }

    @Test func aFailureStillShowsAToast() {
        let center = ToastCenter()
        center.show(.decoding("x"))
        #expect(center.current?.message == "Something went wrong. Try again.")
    }
}
