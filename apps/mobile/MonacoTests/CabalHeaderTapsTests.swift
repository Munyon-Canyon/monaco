import MonacoAPI
import Testing

@testable import Monaco

@MainActor
struct CabalHeaderTapsTests {
    @Test func onlyTheCreatorCanChangeThePicture() {
        #expect(CabalHeaderSlot.canChangePicture(.sample(role: "creator")))
        #expect(!CabalHeaderSlot.canChangePicture(.sample(role: "member")))
        #expect(!CabalHeaderSlot.canChangePicture(.sample(role: nil)))
    }
}
