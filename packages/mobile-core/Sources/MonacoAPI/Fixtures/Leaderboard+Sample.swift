#if DEBUG
import Foundation

extension Components.Schemas.LeaderboardSubject {
    public static func sample(
        id: String, kind: KindPayload = .user, name: String, handle: String? = nil, pictureUrl: String? = nil
    ) -> Self {
        Self(id: id, kind: kind, name: name, handle: handle, pictureUrl: pictureUrl)
    }
}

extension Components.Schemas.LeaderboardRow {
    public static func sample(
        rank: Int32, id: String, name: String, kind: Components.Schemas.LeaderboardSubject.KindPayload = .user,
        valueMicros: Int64 = 100_000_000, pnlMicros: Int64 = 2_000_000, returnBps: Int64? = 200,
        flags: [FlagsPayloadPayload] = []
    ) -> Self {
        Self(
            rank: rank, subject: .sample(id: id, kind: kind, name: name, handle: kind == .user ? "@\(name)" : nil),
            valueMicros: valueMicros, pnlMicros: pnlMicros, returnBps: returnBps, flags: flags)
    }
}

extension Components.Schemas.LeaderboardPage {
    public static let sampleComputedAt = Date(timeIntervalSince1970: 1_790_996_460)

    public static func samplePeople(
        range: RangePayload = .all, firstRank: Int32 = 1, count: Int = 3, nextCursor: String? = nil,
        me: Components.Schemas.LeaderboardRow? = nil
    ) -> Self {
        Self(
            runId: "01890a5d-ac96-774b-bcce-b302099a8060", board: "people", range: range,
            computedAt: sampleComputedAt, pricesAsOf: sampleComputedAt.addingTimeInterval(-60),
            rows: (0..<Int32(count)).map { offset in
                .sample(
                    rank: firstRank + offset, id: "user-\(firstRank + offset)", name: "Investor \(firstRank + offset)",
                    returnBps: Int64(500 - 50 * (firstRank + offset)))
            },
            me: me.map(MePayload.init(row:)), nextCursor: nextCursor)
    }

    public static let sampleEmpty = Self(
        runId: nil, board: "people", range: .all, computedAt: sampleComputedAt, pricesAsOf: sampleComputedAt,
        rows: [], me: nil, nextCursor: nil)
}

extension Components.Schemas.LeaderboardPage.MePayload {
    init(row: Components.Schemas.LeaderboardRow) {
        self.init(
            rank: row.rank, subject: row.subject, valueMicros: row.valueMicros, pnlMicros: row.pnlMicros,
            returnBps: row.returnBps, flags: row.flags.compactMap { .init(rawValue: $0.rawValue) })
    }
}
#endif
