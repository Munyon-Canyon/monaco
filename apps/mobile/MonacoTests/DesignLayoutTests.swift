import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

/// `MoneyFont` builds its `@ScaledMetric` bases from `MoneyStyle.baseSize`, so these assertions
/// are over the numbers that actually render. The values are the redesign's money roles
/// (`docs/design.md`): hero 44 semibold, home 32, display 28 heavy, large 22 bold, card 20, row and caption 15.
struct MoneyStyleScalingTests {
    @Test func everyStyleHasItsDesignSize() {
        #expect(MoneyStyle.hero.baseSize == 44)
        #expect(MoneyStyle.home.baseSize == 32)
        #expect(MoneyStyle.display.baseSize == 28)
        #expect(MoneyStyle.large.baseSize == 22)
        #expect(MoneyStyle.card.baseSize == 20)
        #expect(MoneyStyle.row.baseSize == 15)
        #expect(MoneyStyle.caption.baseSize == 15)
    }

    @Test func stylesScaleAgainstTheTextStyleOfTheirSize() {
        #expect(MoneyStyle.hero.textStyle == .largeTitle)
        #expect(MoneyStyle.home.textStyle == .largeTitle)
        #expect(MoneyStyle.display.textStyle == .title)
        #expect(MoneyStyle.large.textStyle == .title2)
        #expect(MoneyStyle.card.textStyle == .title3)
        #expect(MoneyStyle.row.textStyle == .subheadline)
        #expect(MoneyStyle.caption.textStyle == .subheadline)
    }

    @Test func weightsMatchTheDesign() {
        #expect(MoneyStyle.hero.weight == .semibold)
        #expect(MoneyStyle.home.weight == .semibold)
        #expect(MoneyStyle.display.weight == .heavy)
        #expect(MoneyStyle.large.weight == .bold)
        #expect(MoneyStyle.card.weight == .semibold)
        #expect(MoneyStyle.row.weight == .semibold)
        #expect(MoneyStyle.caption.weight == .semibold)
    }

    @Test func onlyTheDisplayFigureSetsTracking() {
        #expect(MoneyStyle.display.tracking == -0.5)
        #expect(MoneyStyle.home.tracking == 0)
        #expect(MoneyStyle.hero.tracking == 0)
    }
}

struct FTokenTests {
    @Test func spacingMatchesF() {
        #expect(MonacoTheme.Space.gutter == 16)
        #expect(MonacoTheme.Space.section == 16)
        #expect(MonacoTheme.Space.root == 24)
        #expect(MonacoTheme.Space.cardPadding == 12)
        #expect(MonacoTheme.Space.titleGap == 6)
        #expect(MonacoTheme.Space.stackGap == 8)
        #expect(MonacoTheme.Space.tabClear == 104)
        #expect(MonacoTheme.Space.homeClear == 40)
        #expect(SectionHeaderMetrics.topSpacing == MonacoTheme.Space.section)
        #expect(SectionHeaderMetrics.bottomSpacing == MonacoTheme.Space.titleGap)
    }

    @Test func radiiMatchF() {
        #expect(MonacoTheme.Radius.card == 12)
        #expect(MonacoTheme.Radius.cardTight == 12)
        #expect(MonacoTheme.Radius.button == 10)
        #expect(MonacoTheme.Radius.shareCard == 16)
        #expect(MonacoTheme.Radius.small == 8)
        #expect(MonacoTheme.Radius.field == 8)
        #expect(MonacoTheme.Radius.tile == 6)
        #expect(MonacoTheme.Radius.keypadKey == 8)
        #expect(MonacoTheme.Radius.crest == 6)
        #expect(MonacoTheme.Radius.logoFraction == 0.15)
        #expect(MonacoTheme.Radius.squircleFraction == 0.225)
    }

    @Test func reactionChipIsSmallOnTheScreenAndBigUnderTheFinger() {
        let chip = MonacoTheme.ReactionChipMetrics.self
        #expect(chip.visibleHeight == 24)
        #expect(chip.tapSize == 44)
        #expect(chip.minWidth == 44)
        #expect(chip.horizontalPadding == 8)
        #expect(chip.gap == 4)
    }

    @Test func theFeedLogoIs36() {
        #expect(StockMarkSize.feed.rawValue == 36)
    }
}

@MainActor
struct SectionStackLayoutTests {
    @Test func aZeroHeightSectionTakesNoGap() {
        let host = UIHostingController(
            rootView: SectionStackLayout(spacing: SectionStackMetrics.spacing) {
                Color.red.frame(height: 10)
                Color.clear.frame(height: 0)
                Color.red.frame(height: 10)
            })
        let size = host.sizeThatFits(in: CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude))
        #expect(size.height == 20 + SectionStackMetrics.spacing)
    }
}

@MainActor
struct V3LargeTextLayoutTests {
    private static let phone = CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude)

    private static func fitted(_ view: some View, at size: DynamicTypeSize) -> CGSize {
        UIHostingController(rootView: view.environment(\.dynamicTypeSize, size)).sizeThatFits(in: phone)
    }

    private static func card(_ layout: CabalCoverCardLayout) -> CabalCoverCard {
        let model = CabalCoverCardModel(
            id: "cabal-1", name: "Weekend Warriors", memberCountText: "8 members", votesNeededText: "1 vote needed",
            shareText: "$7,000.00", ownershipText: "35.00%",
            pnl: .init(amountText: "−$300.00", percentText: "−4.11%", direction: .loss),
            groupReturn: .init(periodText: "1M", percentText: "−4.12%", direction: .loss), tint: .blue)
        return CabalCoverCard(model: model, layout: layout, onOpen: {}, onShare: {})
    }

    private static let items = [
        StatItem(label: "Units", value: "30.0000"),
        StatItem(label: "Average buy", value: "$412.40"),
        StatItem(label: "Cost", value: "$12,372.00"),
        StatItem(label: "Value", value: "$13,950.00"),
    ]

    @Test(arguments: [DynamicTypeSize.xxLarge, .xxxLarge, .accessibility1, .accessibility5])
    func theRailCardKeepsItsWidthAndNeverShrinksBelowItsDesignHeight(size: DynamicTypeSize) {
        let fitted = Self.fitted(Self.card(.rail), at: size)
        #expect(fitted.width == CabalCoverCardMetrics.railWidth)
        #expect(fitted.height >= CabalCoverCardMetrics.railHeight, "rail card is \(fitted.height)pt at \(size)")
    }

    @Test(arguments: [DynamicTypeSize.accessibility1, .accessibility3, .accessibility5])
    func theCoverCardsReachTheirLargeTextHeightAtAccessibilitySizes(size: DynamicTypeSize) {
        #expect(Self.fitted(Self.card(.rail), at: size).height >= CabalCoverCardMetrics.railLargeTextHeight)
        #expect(Self.fitted(Self.card(.list), at: size).height >= CabalCoverCardMetrics.listLargeTextHeight)
    }

    @Test func theMoneyStripWrapsOnceTheFiguresStopFittingOneLine() {
        let strip = MoneyStrip(netWorth: "$1,234,567.89", cash: "$500,000.00", depositTitle: "Deposit", onDeposit: {})
        let inline = Self.fitted(strip, at: .large).height
        let wrapped = Self.fitted(strip, at: .xxxLarge).height
        #expect(wrapped > inline, "the strip is \(inline)pt at large and \(wrapped)pt at xxxLarge")
    }

    @Test func theStatGridKeepsTwoColumnsUntilAccessibilitySizesThenStacks() {
        let grid = StatGrid(items: Self.items)
        let twoColumns = Self.fitted(grid, at: .xxxLarge).height
        let stacked = Self.fitted(grid, at: .accessibility1).height
        #expect(twoColumns < 4 * StatMetrics.cellMinHeight, "two columns at xxxLarge: \(twoColumns)pt")
        #expect(stacked >= 4 * StatMetrics.cellMinHeight, "one column at accessibility1: \(stacked)pt")
    }

    @Test func theTwoStatRuleStacksAtAccessibility1AndNotBefore() {
        let one = [StatItem(label: "Your share", value: "$7,000.00")]
        let two = one + [StatItem(label: "Cash", value: "$5,100.00")]
        #expect(
            Self.fitted(StatPair(items: two), at: .xxxLarge).height
                == Self.fitted(StatPair(items: one), at: .xxxLarge).height)
        #expect(
            Self.fitted(StatPair(items: two), at: .accessibility1).height
                > Self.fitted(StatPair(items: one), at: .accessibility1).height)
    }

    @Test(arguments: DynamicTypeSize.allCases)
    func theCompactProminentButtonKeepsItsFortyFourPointHitFrame(size: DynamicTypeSize) {
        let button = Button("Deposit") {}.buttonStyle(.monacoCompactProminent)
        let fitted = Self.fitted(button, at: size)
        #expect(fitted.height >= MonacoButtonMetrics.hitTarget, "\(fitted.height)pt at \(size)")
    }

    private struct SegmentedProbe: View {
        let options: [String]
        @State private var selection: String

        init(_ options: [String]) {
            self.options = options
            _selection = State(initialValue: options[0])
        }

        var body: some View {
            MonacoSegmented(options, selection: $selection) { $0 }
                .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }

    @Test(arguments: [["Text message", "Email"], ["Discover", "Following"]])
    func aTwoOptionSegmentedStacksWhenItsLabelsStopFittingOneRow(options: [String]) {
        let probe = SegmentedProbe(options)
        let row = Self.fitted(probe, at: .large).height
        let stacked = Self.fitted(probe, at: .accessibility5).height
        #expect(stacked >= 2 * MonacoSegmentedMetrics.minHeight, "\(stacked)pt at AX5, \(row)pt at large")
        #expect(Self.fitted(probe, at: .xLarge).height == row, "the row must not change below xxLarge")
    }

    private struct RangeProbe: View {
        static let labels = ["1H", "1D", "1W", "1M", "3M", "6M", "All"]

        var body: some View {
            MonacoRangeChips(
                ranges: Self.labels, selection: "1M", identifierPrefix: "probe", label: { $0 }, onSelect: { _ in })
        }
    }

    private static func idealWidth(of text: String, at size: DynamicTypeSize) -> CGFloat {
        let host = UIHostingController(
            rootView: Text(text).font(MonacoTheme.Typo.subhead).fixedSize().environment(\.dynamicTypeSize, size))
        return host.sizeThatFits(in: CGSize(width: 1000, height: 100)).width
    }

    @Test(arguments: [DynamicTypeSize.xxxLarge, .accessibility1, .accessibility5])
    func aSevenLabelRangeRowScrollsInsteadOfStackingOrCuttingALabel(size: DynamicTypeSize) {
        let row = RangeProbe().frame(width: 393)
        let fitted = Self.fitted(row, at: size).height
        #expect(fitted < 2 * MonacoButtonMetrics.hitTarget, "\(fitted)pt at \(size)")
        let ideal = UIHostingController(rootView: RangeProbe().environment(\.dynamicTypeSize, size))
            .sizeThatFits(in: CGSize(width: CGFloat.greatestFiniteMagnitude, height: CGFloat.greatestFiniteMagnitude))
            .width
        let labels = RangeProbe.labels.map { Self.idealWidth(of: $0, at: size) }.reduce(0, +)
        #expect(ideal >= labels, "ideal \(ideal)pt, labels \(labels)pt at \(size)")
    }

    @Test func theSevenLabelRangeRowFitsThePhoneAtLargeSize() {
        #expect(Self.fitted(RangeProbe(), at: .large).width <= 393)
    }
}

@MainActor
struct FLargeTextLayoutRuleTests {
    private static let phone = CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude)

    private static func fitted(_ view: some View, at size: DynamicTypeSize) -> CGSize {
        UIHostingController(rootView: view.environment(\.dynamicTypeSize, size)).sizeThatFits(in: phone)
    }

    private static func yesNo() -> some View {
        VoteHoldRow(yes: Color.red.frame(height: 50), no: Color.blue.frame(height: 50))
    }

    @Test func theVotePairStacksAtAccessibility1AndSitsSideBySideBefore() {
        #expect(Self.fitted(Self.yesNo(), at: .large).height == 50)
        #expect(Self.fitted(Self.yesNo(), at: .accessibility1).height > 50)
        #expect(Self.fitted(Self.yesNo(), at: .accessibility5).height > 50)
    }

    @Test func theStatRowGoesFourToTwoByTwoAtXXLargeAndOnePerRowAtAccessibility1() {
        let requested = StatMetrics.defaultColumns
        #expect(StatMetrics.columns(requested: requested, dynamicTypeSize: .xLarge) == 4)
        #expect(StatMetrics.columns(requested: requested, dynamicTypeSize: .xxLarge) == 2)
        #expect(StatMetrics.columns(requested: requested, dynamicTypeSize: .xxxLarge) == 2)
        #expect(StatMetrics.columns(requested: requested, dynamicTypeSize: .accessibility1) == 1)
        #expect(StatMetrics.columns(requested: requested, dynamicTypeSize: .accessibility5) == 1)
    }

    @Test func aRankedRowStacksNameOverRateAtAccessibility1AndNotBefore() {
        #expect(!MonacoRowLayout(dynamicTypeSize: .xxxLarge).isStacked)
        #expect(MonacoRowLayout(dynamicTypeSize: .accessibility1).isStacked)
        #expect(MonacoRowLayout(dynamicTypeSize: .accessibility5).isStacked)
    }

    @Test func aSheetScrollsItsContentAtAccessibilitySizesEvenWhenFixed() {
        #expect(!MonacoSheetDetents.scrollsContent(declared: false, dynamicTypeSize: .xxxLarge))
        #expect(MonacoSheetDetents.scrollsContent(declared: false, dynamicTypeSize: .accessibility1))
        #expect(MonacoSheetDetents.scrollsContent(declared: true, dynamicTypeSize: .large))
    }

    @Test func aToastWrapsWithoutALineLimitAtAccessibilitySizes() {
        #expect(MonacoToastLayout.lineLimit(.large) == 2)
        #expect(MonacoToastLayout.lineLimit(.accessibility1) == nil)
        #expect(MonacoToastLayout.lineLimit(.accessibility5) == nil)
    }

    @Test func theAmountChipsFallIntoTwoByTwoAtAccessibility1() {
        let stacked = DynamicTypeSize.accessibility1.isAccessibilitySize
        #expect(AmountText.presetRows(4, stacked: stacked) == [[0, 1], [2, 3]])
        #expect(AmountText.presetRows(4, stacked: DynamicTypeSize.xxxLarge.isAccessibilitySize) == [[0, 1, 2, 3]])
    }

    private struct TabsProbe: View {
        @State private var selection = "Treasury"
        var body: some View {
            MonacoUnderlineTabs(["Treasury", "Rules", "Members", "Activity"], selection: $selection) { $0 }
        }
    }

    @Test(arguments: [DynamicTypeSize.large, .xxxLarge, .accessibility1, .accessibility5])
    func underlineTabsStayOneRowAndScrollInsteadOfWrapping(size: DynamicTypeSize) {
        let height = Self.fitted(TabsProbe(), at: size).height
        #expect(height >= MonacoUnderlineTabsMetrics.minHeight, "\(height)pt at \(size)")
        #expect(height < 2 * MonacoUnderlineTabsMetrics.minHeight + 16, "\(height)pt at \(size)")
    }

    @Test(arguments: [MonacoSheetDetents.review, .votes, .result, .fitted])
    func theSheetHelperAddsLargeOnlyAtAccessibilitySizes(sheet: MonacoSheetDetents) {
        let screen: CGFloat = 800
        let regular = sheet.detents(fittedHeight: 200, screenHeight: screen, dynamicTypeSize: .xxxLarge)
        let large = sheet.detents(fittedHeight: 200, screenHeight: screen, dynamicTypeSize: .accessibility1)
        #expect(!regular.contains(.large))
        #expect(large == regular + [.large])
    }

    @Test(arguments: DynamicTypeSize.allCases)
    func theRemindControlKeepsItsFortyFourPointHit(size: DynamicTypeSize) {
        let control = RemindControl(isOn: false, onTitle: "Reminder set", action: {})
        #expect(Self.fitted(control, at: size).height >= RemindMetrics.target, "at \(size)")
        #expect(RemindMetrics.target == 44)
    }
}

struct RangeChipsLargeTextTests {
    @Test func rangeChipsScrollAtEveryAccessibilitySizeAndOnlyThere() {
        for size in DynamicTypeSize.allCases {
            #expect(MonacoPlainLabelMetrics.scrollsAlways(size) == size.isAccessibilitySize)
        }
    }
}
