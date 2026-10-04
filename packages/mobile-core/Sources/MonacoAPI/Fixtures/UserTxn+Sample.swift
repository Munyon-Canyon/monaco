#if DEBUG
import Foundation

extension Components.Schemas.UserTxn {
    public static let sample = sampleTxn(1, sampleCycle[0], daysAgo: 0)

    fileprivate typealias Pattern = (
        kind: KindPayload, status: StatusPayload, micros: String, cabal: CabalPayload?
    )

    fileprivate static let sampleCabal = CabalPayload(id: "01890a5d-ac96-774b-bcce-b302099a8060", name: "QA pot")

    fileprivate static let sampleCycle: [Pattern] = [
        (.deposit, .settled, "25000000", nil),
        (.fund, .settled, "-10000000", sampleCabal),
        (.withdrawal, .pending, "-5000000", nil),
        (.cashOut, .settled, "3500000", sampleCabal),
        (.fund, .failed, "-2000000", sampleCabal),
        (.deposit, .settled, "100000000", nil),
        (.fund, .settled, "-50000000", sampleCabal),
        (.deposit, .settled, "12500000", nil),
        (.fund, .settled, "-12500000", sampleCabal),
        (.withdrawal, .settled, "-30000000", nil),
    ]

    fileprivate static func sampleTxn(_ number: Int, _ pattern: Pattern, daysAgo: Int) -> Self {
        let seconds = 1_790_996_400 - daysAgo * 86_400 - number * 60
        return Self(
            id: String(format: "01890a5d-ac96-774b-bcce-b302099a8%03d", number), kind: pattern.kind,
            status: pattern.status, usdcMicros: pattern.micros, cabal: pattern.cabal,
            txSignature: pattern.status == .pending ? nil : "sample-signature-\(number)",
            createdAt: Date(timeIntervalSince1970: TimeInterval(seconds)))
    }
}

extension Components.Schemas.UserTxnPage {
    public static let sampleCursor = "sample-cursor-2"

    public static let sampleFirst = Self(
        items: (1...30).map {
            .sampleTxn($0, Components.Schemas.UserTxn.sampleCycle[($0 - 1) % 10], daysAgo: ($0 - 1) / 3)
        }, nextCursor: sampleCursor)

    public static let sampleSecond = Self(
        items: [
            .sampleTxn(31, (.fund, .settled, "-8000000", nil), daysAgo: 11),
            .sampleTxn(32, (.cashOut, .settled, "1500000", nil), daysAgo: 12),
            .sampleTxn(33, (.deposit, .settled, "60000000", nil), daysAgo: 400),
            .sampleTxn(34, (.withdrawal, .failed, "-9000000", nil), daysAgo: 401),
        ], nextCursor: nil)

    public static let sampleEmpty = Self(items: [], nextCursor: nil)
}
#endif
