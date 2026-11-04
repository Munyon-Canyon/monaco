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

    @Test func foldedTitleLabelReadsShortNameAndMemberCount() {
        let eight = CabalFoldedTitle.accessibilityLabel(name: "AC Capital Group", memberCount: 8)
        let one = CabalFoldedTitle.accessibilityLabel(name: "AC Capital", memberCount: 1)
        #expect(eight == "AC Capital, 8 members")
        #expect(one == "AC Capital, 1 member")
    }
}
