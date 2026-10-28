import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct RankedRowTests {
    private static func row(
        rank: RankedRow<Color>.Rank = .ranked(8, movement: 1), name: String = "Andres", isYou: Bool = true,
        subtitle: String = "Voted 11 of 11", figure: String = "78%", detail: String = "7 of 9 right",
        isYouBand: Bool? = nil
    ) -> RankedRow<Color> {
        RankedRow(
            rank: rank, name: name, isYou: isYou, subtitle: subtitle, figure: figure, detail: detail,
            isYouBand: isYouBand
        ) { Color.gray }
    }

    private static func fitted(_ view: some View, at size: DynamicTypeSize = .large) -> CGSize {
        UIHostingController(rootView: view.environment(\.dynamicTypeSize, size))
            .sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude))
    }

    @Test func aRankedRowYouReadsAsOneSentence() {
        let spoken = RankedRowSpeech.spoken(
            rank: .ranked(8, movement: 1), name: "Andres", isYou: true, subtitle: "Voted 11 of 11", figure: "78%",
            detail: "7 of 9 right")

        #expect(spoken == "8th, up 1. Andres, you. 78 percent, 7 of 9 right. Voted 11 of 11.")
    }

    @Test func aRankedRowThatMovedDownOrNotAtAllSaysSo() {
        let down = RankedRowSpeech.spoken(
            rank: .ranked(9, movement: -1), name: "Rosa", isYou: false, subtitle: "Voted 8 of 9", figure: "71%",
            detail: "5 of 7 right")
        let same = RankedRowSpeech.spoken(
            rank: .ranked(3, movement: 0), name: "Kai", isYou: false, subtitle: "Voted 8 of 9", figure: "71%",
            detail: "5 of 7 right")

        #expect(down == "9th, down 1. Rosa. 71 percent, 5 of 7 right. Voted 8 of 9.")
        #expect(same == "3rd. Kai. 71 percent, 5 of 7 right. Voted 8 of 9.")
    }

    @Test func anUnrankedRowSaysNotRankedAndHowManyMoreToRank() {
        let spoken = RankedRowSpeech.spoken(
            rank: .unranked, name: "Rosa", isYou: false, subtitle: "2 resolved · voted 3 of 4", figure: "3 more",
            detail: "to rank")

        #expect(spoken.hasPrefix("Not ranked. Rosa. 3 more to rank."))
        #expect(spoken.hasSuffix("2 resolved · voted 3 of 4."))
    }

    @Test func theMovementGlyphFollowsTheSign() {
        #expect(RankedRowSpeech.glyph(for: 1) == .up(1))
        #expect(RankedRowSpeech.glyph(for: 3) == .up(3))
        #expect(RankedRowSpeech.glyph(for: -1) == .down(1))
        #expect(RankedRowSpeech.glyph(for: 0) == .none)
        #expect(RankedRowSpeech.glyph(for: nil) == .none)
    }

    @Test func theBandFollowsIsYouUnlessTheCallerSaysOtherwise() {
        #expect(Self.row(isYou: true).isYouBand)
        #expect(!Self.row(isYou: false).isYouBand)
        #expect(!Self.row(isYou: true, isYouBand: false).isYouBand)
        #expect(Self.row(isYou: false, isYouBand: true).isYouBand)
    }

    @Test(arguments: [DynamicTypeSize.large, .accessibility3])
    func theRowIsAtLeastSixtyTall(size: DynamicTypeSize) {
        #expect(Self.fitted(Self.row(), at: size).height >= 60)
        #expect(Self.fitted(Self.row(rank: .unranked, figure: "3 more", detail: "to rank"), at: size).height >= 60)
    }

    @Test func theTrailingPairStacksUnderTheLabelsAtAccessibility3() async throws {
        let inline = Self.fitted(Self.row()).height
        let stacked = Self.fitted(Self.row(), at: .accessibility3).height
        let labels = try await RenderedView.labels(Self.row(), size: .accessibility3) { !$0.isEmpty }

        #expect(stacked > inline + 40, "inline \(inline), stacked \(stacked)")
        #expect(labels.contains { $0.contains("78 percent, 7 of 9 right") }, "labels: \(labels)")
        #expect(labels.count == 1, "labels: \(labels)")
    }
}
