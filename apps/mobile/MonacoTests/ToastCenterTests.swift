import Foundation
import MonacoAPI
import Testing

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
}
