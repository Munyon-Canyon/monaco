import MonacoAPI
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct CabalHeaderTapsTests {
    @Test func onlyTheCreatorCanChangeThePicture() {
        #expect(CabalHeaderSlot.canChangePicture(.sample(role: "creator")))
        #expect(!CabalHeaderSlot.canChangePicture(.sample(role: "member")))
        #expect(!CabalHeaderSlot.canChangePicture(.sample(role: nil)))
    }

    @Test func coverHeightIs136() {
        #expect(CabalPageMetrics.coverHeight == 136)
    }

    @Test func insetCoverUsesCardRadiusAndNoScrim() {
        #expect(CabalCover<EmptyView>.cornerRadius(for: .heroInset) == MonacoTheme.Radius.card)
        #expect(CabalCover<EmptyView>.overlay(for: .heroInset, reduceTransparency: false) == nil)
        #expect(CabalCover<EmptyView>.overlay(for: .heroInset, reduceTransparency: true) == nil)
    }
}
