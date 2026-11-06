import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ProposeReviewThesisTests {
    private let writingDetents: [PresentationDetent] = [.medium, .large]

    @Test func theWritingReviewSheetOffersHalfAndFullHeight() {
        #expect(MonacoSheetDetents.reviewWriting.detents(fittedHeight: 0) == [.medium, .large])
        #expect(
            MonacoSheetDetents.reviewWriting.detents(fittedHeight: 0, dynamicTypeSize: .accessibility2) == [
                .medium, .large,
            ])
    }

    @Test func theReviewSheetWithoutWritingStaysAtHalf() {
        #expect(MonacoSheetDetents.review.detents(fittedHeight: 0) == [.medium])
    }

    @Test func focusingTheBoxTakesTheSheetToFullHeight() {
        let focused = MonacoSheetDetents.selected(
            expanded: true, chosen: nil, in: writingDetents, dynamicTypeSize: .large)
        #expect(focused == .large)
    }

    @Test func losingFocusReturnsTheSheetToHalf() {
        let resting = MonacoSheetDetents.selected(
            expanded: false, chosen: nil, in: writingDetents, dynamicTypeSize: .large)
        #expect(resting == .medium)
    }

    @Test func aDetentTheSheetCannotShowIsNotForcedOnIt() {
        let half = MonacoSheetDetents.selected(
            expanded: true, chosen: nil, in: [.medium], dynamicTypeSize: .large)
        #expect(half == .medium)
    }

    @Test func aDraggedDetentWinsWhileNothingIsFocused() {
        let dragged = MonacoSheetDetents.selected(
            expanded: false, chosen: .large, in: writingDetents, dynamicTypeSize: .large)
        #expect(dragged == .large)
    }

    @Test func accessibilitySizesOpenAtFullHeightEvenWhenResting() {
        let resting = MonacoSheetDetents.selected(
            expanded: false, chosen: nil, in: writingDetents, dynamicTypeSize: .accessibility1)
        #expect(resting == .large)
    }

    @Test func theCounterIsReadAsWords() {
        #expect(ProposeReviewScreen.counterSpeech("197 / 280") == "197 of 280 characters")
    }

    @Test func theHintSaysOptionalAndTheLimit() {
        #expect(ProposeReviewScreen.thesisHint == "Optional. Up to 280 characters.")
    }

    @Test func theFoldedMarkIs24Points() {
        #expect(ProposeReviewScreen.foldedMarkSize == 24)
        #expect(ProposeReviewScreen.avatarSize == 32)
    }
}
