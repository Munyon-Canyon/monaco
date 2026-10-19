import SwiftUI
import Testing
import UIKit

@testable import Monaco

struct CircleActionMetricsTests {
    @Test func theDiscStopsBeforeItOutgrowsItsColumn() {
        let narrowestColumn: CGFloat = 362 / 3
        #expect(CircleActionMetrics.maximumDiscSize < narrowestColumn)

        for scaled in [CGFloat(99), 189] {
            #expect(CircleActionMetrics.discSize(scaled: scaled) < narrowestColumn)
            #expect(CircleActionMetrics.discSize(scaled: scaled) == CircleActionMetrics.maximumDiscSize)
        }
    }

    @Test func theGlyphStopsWithTheDisc() {
        #expect(CircleActionMetrics.glyphSize(scaled: 68) == CircleActionMetrics.maximumGlyphSize)
        #expect(CircleActionMetrics.maximumGlyphSize < CircleActionMetrics.maximumDiscSize)
    }

    @Test func bothStillScaleUpToTheirCeiling() {
        #expect(CircleActionMetrics.baseDiscSize == 44)
        #expect(CircleActionMetrics.maximumDiscSize == 64)
        #expect(CircleActionMetrics.discSize(scaled: 44) == 44)
        #expect(CircleActionMetrics.discSize(scaled: 60) == 60)
        #expect(CircleActionMetrics.glyphSize(scaled: 22) == 22)
        #expect(CircleActionMetrics.glyphSize(scaled: 26) == 26)
    }

    @Test func threeTilesFitTheContentWidth() {
        let contentWidth: CGFloat = 362
        #expect(CircleActionMetrics.minimumTileWidth * 3 <= contentWidth)
        #expect(CircleActionMetrics.minimumTileHeight == 72)
    }
}

@MainActor
struct MonacoCompactButtonLayoutTests {
    private func fitted<S: ButtonStyle>(_ style: S, at size: DynamicTypeSize) -> CGSize {
        let host = UIHostingController(
            rootView: Button("Deposit") {}.buttonStyle(style).environment(\.dynamicTypeSize, size))
        return host.sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude))
    }

    @Test(arguments: DynamicTypeSize.allCases)
    func theDepositCapsuleKeepsItsHitFrameAtEveryTextSize(size: DynamicTypeSize) {
        let hit = fitted(.monacoCompactProminent, at: size)
        #expect(hit.height >= MonacoButtonMetrics.hitTarget)
        #expect(hit.width >= MonacoButtonMetrics.compactMinWidth)
    }

    @Test(arguments: DynamicTypeSize.allCases)
    func theSmallButtonKeepsItsHitFrameAtEveryTextSize(size: DynamicTypeSize) {
        #expect(fitted(.monacoCompact, at: size).height >= MonacoButtonMetrics.hitTarget)
    }

    @Test func theRegularButtonsAreFortyEightPointsTall() {
        #expect(MonacoButtonMetrics.minimumHeight == 48)
        #expect(fitted(.monacoPrimary, at: .large).height >= MonacoButtonMetrics.minimumHeight)
        #expect(fitted(.monacoOutline, at: .large).height >= MonacoButtonMetrics.minimumHeight)
    }
}

@MainActor
struct BottomCTALayoutTests {
    @Test func theBarSpansTheContainerWhateverItsContent() {
        let host = UIHostingController(rootView: BottomCTA { Text("x") })
        let size = host.sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude))
        #expect(size.width == 390)
    }

    private func height(at size: DynamicTypeSize) -> CGFloat {
        let host = UIHostingController(
            rootView: BottomCTA {
                Button("Propose buy of this stock") {}.buttonStyle(.monacoPrimary)
                Button("Propose sell of this stock") {}.buttonStyle(.monacoSecondary)
            }
            .dynamicTypeSize(size))
        return host.sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude)).height
    }

    @Test func twoButtonsStackAtAccessibilitySizes() {
        #expect(height(at: .accessibility3) >= height(at: .large) + MonacoButtonMetrics.minimumHeight)
    }
}

@MainActor
struct MonacoSegmentedLayoutTests {
    private func controlHeight(
        _ labels: [String], width: CGFloat, size: DynamicTypeSize
    ) -> CGFloat {
        let host = UIHostingController(
            rootView: MonacoSegmented(labels, selection: .constant(labels[0])) { $0 }
                .environment(\.dynamicTypeSize, size)
                .frame(width: width))
        return host.sizeThatFits(in: CGSize(width: width, height: CGFloat.greatestFiniteMagnitude)).height
    }

    @Test func theTrackIsAtLeastFortyTwoPointsTall() {
        #expect(controlHeight(["Email", "Text"], width: 320, size: .large) >= MonacoSegmentedMetrics.minHeight)
    }

    @Test func aLongLabelWrapsInsteadOfTruncatingAtAccessibilitySizes() {
        let long = controlHeight(["Text message", "Email"], width: 320, size: .accessibility3)
        let short = controlHeight(["Email", "Text"], width: 320, size: .accessibility3)
        #expect(long > short)
    }
}

@MainActor
struct BottomCTAStackingTests {
    private func height<Content: View>(at size: DynamicTypeSize, @ViewBuilder _ content: () -> Content) -> CGFloat {
        let host = UIHostingController(
            rootView: BottomCTA(content: content).environment(\.dynamicTypeSize, size))
        return host.sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude)).height
    }

    @Test func aPairIsOneButtonTallerThanALoneButtonAtAccessibilitySizes() {
        let alone = height(at: .accessibility3) {
            Button("Propose buy") {}.buttonStyle(.monacoPrimary)
        }
        let pair = height(at: .accessibility3) {
            Button("Propose buy") {}.buttonStyle(.monacoPrimary)
            Button("Propose sell") {}.buttonStyle(.monacoSecondary)
        }
        #expect(pair >= alone + MonacoButtonMetrics.minimumHeight)
    }

    @Test func twoButtonsStaySideBySideAtLargeSize() {
        let alone = height(at: .large) {
            Button("Propose buy") {}.buttonStyle(.monacoPrimary)
        }
        let pair = height(at: .large) {
            Button("Propose buy") {}.buttonStyle(.monacoPrimary)
            Button("Propose sell") {}.buttonStyle(.monacoSecondary)
        }
        #expect(pair == alone)
    }
}
