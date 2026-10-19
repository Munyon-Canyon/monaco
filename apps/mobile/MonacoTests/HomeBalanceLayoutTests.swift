import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct HomeBalanceLayoutTests {
    enum Phone: String, CaseIterable {
        case iPhoneSE, iPhone18ProMax

        var size: CGSize {
            switch self {
            case .iPhoneSE: CGSize(width: 375, height: 667)
            case .iPhone18ProMax: CGSize(width: 440, height: 956)
            }
        }
    }

    static let figures: [(micros: Int64, text: String)] = [
        (0, "$0.00"), (1_990_000, "$1.99"), (12_345_670_000, "$12,345.67"),
    ]

    @Test(.timeLimit(.minutes(1)), arguments: Phone.allCases)
    func theTwoButtonsShareOneLineAtEqualWidth(phone: Phone) async throws {
        try ProposeAmountKeypadTests.setAutomation(enabled: true)
        defer { try? ProposeAmountKeypadTests.setAutomation(enabled: false) }
        let window = try Self.window(HomeBalanceSummary(display: .amount(1_990_000)), size: phone.size)
        defer { window.isHidden = true }

        let (add, withdraw) = try await Self.buttons(in: window)
        #expect(abs(add.width - withdraw.width) < 0.5, "Add money \(add.width)pt, Withdraw \(withdraw.width)pt")
        #expect(abs(add.minY - withdraw.minY) < 0.5, "Add money at y \(add.minY), Withdraw at y \(withdraw.minY)")
        #expect(add.maxX < withdraw.minX, "Add money leads Withdraw")
        #expect(add.height >= 44, "Add money is \(add.height)pt tall")
        #expect(withdraw.height >= 44, "Withdraw is \(withdraw.height)pt tall")
        #expect(abs(add.minX - MonacoTheme.Space.gutter) < 0.5, "Add money starts at \(add.minX), not on the gutter")
        #expect(
            abs(withdraw.maxX - (phone.size.width - MonacoTheme.Space.gutter)) < 0.5,
            "Withdraw ends at \(withdraw.maxX), not on the gutter")
    }

    @Test(.timeLimit(.minutes(1)), arguments: Phone.allCases)
    func everyFigureFitsInsideTheGutter(phone: Phone) async throws {
        try ProposeAmountKeypadTests.setAutomation(enabled: true)
        defer { try? ProposeAmountKeypadTests.setAutomation(enabled: false) }
        for figure in Self.figures {
            let window = try Self.window(HomeBalanceSummary(display: .amount(figure.micros)), size: phone.size)
            defer { window.isHidden = true }
            let value = try await Self.element("platform-balance-value", in: window)
            #expect(value.accessibilityLabel == figure.text)
            let frame = value.accessibilityFrame
            #expect(frame.minX >= MonacoTheme.Space.gutter, "\(figure.text) starts at \(frame.minX)")
            #expect(
                frame.maxX <= phone.size.width - MonacoTheme.Space.gutter, "\(figure.text) ends at \(frame.maxX)")
        }
    }

    @Test(.timeLimit(.minutes(1)), arguments: Phone.allCases)
    func theMoneyStripKeepsDepositAtTheTrailingGutterWithAFullHitTarget(phone: Phone) async throws {
        try ProposeAmountKeypadTests.setAutomation(enabled: true)
        defer { try? ProposeAmountKeypadTests.setAutomation(enabled: false) }
        let strip = HomeMoneyStrip(netWorthMicros: 11_582_320_000, display: .amount(500_000_000))
        let window = try Self.window(strip, size: phone.size)
        defer { window.isHidden = true }

        let deposit = try await Self.element("home-add-money-link", in: window).accessibilityFrame
        let cashElement = try await Self.element("platform-balance-value", in: window)
        let cash = cashElement.accessibilityFrame
        #expect(cashElement.accessibilityValue == "$500.00")
        #expect(deposit.height >= 44, "Deposit is \(deposit.height)pt tall")
        #expect(deposit.width >= 82, "Deposit is \(deposit.width)pt wide")
        #expect(
            abs(deposit.maxX - (phone.size.width - MonacoTheme.Space.gutter)) < 0.5,
            "Deposit ends at \(deposit.maxX), not on the gutter")
        #expect(cash.maxX <= deposit.minX, "Cash \(cash.maxX) runs under Deposit \(deposit.minX)")
    }

    private static func buttons(in window: UIWindow) async throws -> (add: CGRect, withdraw: CGRect) {
        let add = try await element("home-add-money-link", in: window).accessibilityFrame
        let withdraw = try await element("home-withdraw-link", in: window).accessibilityFrame
        return (add, withdraw)
    }

    private static func element(_ identifier: String, in window: UIWindow) async throws -> NSObject {
        let found = await ProposeAmountKeypadTests.eventually(within: 10) {
            ProposeAmountKeypadTests.element(identifier, in: window) != nil
        }
        try #require(found, "no \(identifier) on screen")
        return try #require(ProposeAmountKeypadTests.element(identifier, in: window))
    }

    private static func window(_ summary: some View, size: CGSize) throws -> UIWindow {
        let scene = try #require(
            UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first,
            "the test host has no window scene")
        let window = UIWindow(windowScene: scene)
        window.rootViewController = UIHostingController(
            rootView: NavigationStack { ScrollView { summary } }.dynamicTypeSize(.large))
        window.frame = CGRect(origin: .zero, size: size)
        window.makeKeyAndVisible()
        window.layoutIfNeeded()
        return window
    }
}
