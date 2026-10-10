import MonacoAnalytics
import XCTest

final class AnalyticsPIITests: XCTestCase {
    private let walletAddress = String(repeating: "A", count: 44)

    func testDropsEveryPIIKey() {
        let input: [String: AnalyticsValue] = [
            "email": "a", "phone": "b", "x_handle": "c", "handle": "d", "display_name": "e", "wallet": "f",
            "address": "g", "auth_state": "CREATED",
        ]
        XCTAssertEqual(AnalyticsPII.scrub(input), ["auth_state": "CREATED"])
    }

    func testKeyMatchIgnoresCaseAndHyphens() {
        let input: [String: AnalyticsValue] = ["Email": "a", "X-Handle": "b", "wallet_address": "c", "cabal_count": 2]
        XCTAssertEqual(AnalyticsPII.scrub(input), ["cabal_count": 2])
    }

    func testDropsEmailValues() {
        for value in ["ada@example.com", "mail ada@example.com now", "<ada@mail.example.co>"] {
            XCTAssertEqual(AnalyticsPII.scrub(["note": .string(value)]), [:], value)
        }
    }

    func testDropsE164PhoneValues() {
        for value in ["+14155550123", "call +442071838750.", "+8613800138000"] {
            XCTAssertEqual(AnalyticsPII.scrub(["note": .string(value)]), [:], value)
        }
    }

    func testDropsBase58KeyValues() {
        let thirtyTwo = String(repeating: "B", count: 32)
        for value in [walletAddress, thirtyTwo, "to \(walletAddress)"] {
            XCTAssertEqual(AnalyticsPII.scrub(["note": .string(value)]), [:], value)
        }
    }

    func testKeepsOrdinaryValues() {
        let input: [String: AnalyticsValue] = [
            "flow_id": "3f0c7a52-5a0e-4a41-9d3c-0d7a4b1e9c10",
            "screen": "cabal_detail",
            "login_provider": "google",
            "cabal_count": 3,
            "ratio": .double(0.5),
            "is_new": true,
            "short_plus": "+1234",
            "too_short": .string(String(repeating: "B", count: 31)),
            "too_long": .string(String(repeating: "B", count: 45)),
            "has_zero": .string(String(repeating: "0", count: 40)),
            "incomplete_text": "ada@",
        ]
        XCTAssertEqual(AnalyticsPII.scrub(input), input)
    }
}
