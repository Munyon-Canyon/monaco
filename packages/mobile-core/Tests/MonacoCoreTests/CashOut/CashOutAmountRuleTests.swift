import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class CashOutAmountRuleTests: XCTestCase {
    private let floor: Int64 = 100_000
    private let slice: Int64 = 50_000_000

    private func verdict(_ entered: Int64) -> CashOutAmountRule.Verdict {
        CashOutAmountRule.verdict(enteredMicros: entered, sliceMicros: slice, minimumMicros: floor)
    }

    func testAPartialSaleThatLeavesAWorkableRemainderSellsTheTypedDollars() {
        XCTAssertEqual(verdict(20_000_000), .ok)
        XCTAssertEqual(CashOutAmountRule.sale(for: .ok, enteredMicros: 20_000_000), .usdc(20_000_000))
        XCTAssertEqual(
            CashOutAmountRule.submitTitle(for: .ok, enteredMicros: 20_000_000, sliceMicros: slice), "Cash out $20.00")
    }

    func testNothingTypedIsNotASale() {
        XCTAssertEqual(verdict(0), .noAmount)
        XCTAssertFalse(CashOutAmountRule.Verdict.noAmount.maySubmit)
        XCTAssertNil(CashOutAmountRule.problem(for: .noAmount))
        XCTAssertEqual(CashOutAmountRule.submitTitle(for: .noAmount, enteredMicros: 0, sliceMicros: slice), "Cash out")
    }

    func testUnderTheFloorIsTooSmall() {
        XCTAssertEqual(verdict(50_000), .belowMinimum)
        XCTAssertFalse(CashOutAmountRule.Verdict.belowMinimum.maySubmit)
        XCTAssertEqual(CashOutAmountRule.problem(for: .belowMinimum), "Too small to cash out")
        XCTAssertNil(CashOutAmountRule.sale(for: .belowMinimum, enteredMicros: 50_000))
    }

    func testOverTheSliceIsRefusedWithoutAProblemLine() {
        XCTAssertEqual(verdict(slice + 1), .overSlice)
        XCTAssertFalse(CashOutAmountRule.Verdict.overSlice.maySubmit)
        XCTAssertNil(CashOutAmountRule.problem(for: .overSlice))
        XCTAssertNil(CashOutAmountRule.sale(for: .overSlice, enteredMicros: slice + 1))
    }

    func testAnAmountThatStrandsDustIsPromotedToTheWholeSlice() {
        let entered = slice - (floor - 1)
        let promoted = verdict(entered)

        XCTAssertEqual(promoted, .sellsWholeSlice)
        XCTAssertEqual(CashOutAmountRule.sale(for: promoted, enteredMicros: entered), .all)
        XCTAssertEqual(
            CashOutAmountRule.effectiveMicros(for: promoted, enteredMicros: entered, sliceMicros: slice), slice)
        XCTAssertEqual(
            CashOutAmountRule.submitTitle(for: promoted, enteredMicros: entered, sliceMicros: slice), "Cash out $50.00")
        XCTAssertEqual(
            CashOutAmountRule.helper(for: promoted, sliceMicros: slice), "We'll cash out your whole slice, $50.00")
        XCTAssertTrue(CashOutAmountRule.explainer(for: promoted).contains("your whole slice"))
    }

    func testARemainderExactlyAtTheFloorStaysPartial() {
        XCTAssertEqual(verdict(slice - floor), .ok)
        XCTAssertEqual(CashOutAmountRule.helper(for: .ok, sliceMicros: slice), "Your slice is worth $50.00")
        XCTAssertTrue(CashOutAmountRule.explainer(for: .ok).contains("this much of your slice"))
    }

    func testTheWholeSliceIsAFullExit() {
        XCTAssertEqual(verdict(slice), .sellsWholeSlice)
    }

    func testASliceUnderTheFloorHasNoAmountToType() {
        XCTAssertTrue(CashOutAmountRule.sliceIsBelowMinimum(sliceMicros: 50_000, minimumMicros: floor))
        XCTAssertFalse(CashOutAmountRule.sliceIsBelowMinimum(sliceMicros: floor, minimumMicros: floor))
        XCTAssertFalse(CashOutAmountRule.sliceIsBelowMinimum(sliceMicros: 0, minimumMicros: floor))
        XCTAssertEqual(
            CashOutAmountRule.verdict(enteredMicros: 50_000, sliceMicros: 50_000, minimumMicros: floor), .belowMinimum)
    }

    func testTheSaleEncodesAsAllOrUSDCMicros() throws {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]

        let all = try encoder.encode(CashOutAmountRule.Sale.all.request)
        let dollars = try encoder.encode(CashOutAmountRule.Sale.usdc(1_000_000).request)

        XCTAssertEqual(String(decoding: all, as: UTF8.self), #"{"all":true}"#)
        XCTAssertEqual(String(decoding: dollars, as: UTF8.self), #"{"usdc_micros":"1000000"}"#)
    }
}
