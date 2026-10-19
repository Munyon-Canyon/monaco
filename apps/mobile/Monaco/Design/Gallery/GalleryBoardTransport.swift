#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

struct GalleryBoardTransport: ClientTransport {
    enum Behavior: Sendable {
        case answer(Data)
        case hang
        case fail
    }

    let behavior: Behavior

    func send(
        _ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        switch behavior {
        case .answer(let data):
            var headers = HTTPFields()
            headers[.contentType] = "application/json"
            return (HTTPResponse(status: .ok, headerFields: headers), HTTPBody(data))
        case .hang:
            try await Task.sleep(for: .seconds(3600))
            throw CancellationError()
        case .fail:
            throw URLError(.notConnectedToInternet)
        }
    }

    static func page(rows count: Int, hasMore: Bool, viewerRank: Int32? = nil) -> GalleryBoardTransport {
        let rows = (0..<count).map { index -> Components.Schemas.LeaderboardRow in
            let offset = Int64(index)
            return .sample(
                rank: Int32(index + 1), id: "gallery-cabal-\(index + 1)", name: cabalName(index), kind: .cabal,
                valueMicros: 9_800_000_000 - offset * 1_300_000_000,
                pnlMicros: 1_200_000_000 - offset * 700_000_000, returnBps: 1_800 - offset * 650)
        }
        let viewer = viewerRank.map {
            Components.Schemas.LeaderboardPage.MePayload(
                rank: $0, subject: .sample(id: "gallery-cabal-viewer", kind: .cabal, name: "Weekend investors"),
                valueMicros: 2_100_000_000, pnlMicros: -140_000_000, returnBps: -620, flags: [])
        }
        let page = Components.Schemas.LeaderboardPage(
            runId: "01890a5d-ac96-774b-bcce-b302099a8060", board: "cabals", range: .all, computedAt: .now,
            pricesAsOf: .now, rows: rows, me: viewer, nextCursor: hasMore ? "gallery-next" : nil)
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return GalleryBoardTransport(behavior: .answer((try? encoder.encode(page)) ?? Data("{}".utf8)))
    }

    private static func cabalName(_ index: Int) -> String {
        let names = ["Semis or bust", "Index huggers", "Long shots", "Sunday club", "Dip buyers"]
        return names[index % names.count]
    }
}

struct GalleryBoard: View {
    let caption: String
    @State private var loader: LeaderboardLoader

    init(caption: String, transport: GalleryBoardTransport) {
        self.caption = caption
        let api = APIClient(
            serverURL: URL(filePath: "/gallery"),
            tokens: SessionTokens(privyToken: { "gallery-token" }, refresh: { _ in nil }), transport: transport)
        _loader = State(
            initialValue: LeaderboardLoader(board: .cabals, api: api, hints: SampleSilentHints()))
    }

    var body: some View {
        GalleryCase(caption: caption) {
            VStack(spacing: 0) {
                LeaderboardBoardList(
                    loader: loader, skeletonRows: 2, failureThing: "the board", identifier: "gallery-board"
                ) { row, isLast in
                    BoardRow(row: row, figure: .value, isLast: isLast, chevron: true) {
                        CabalMark(groupId: row.id, name: row.name, size: BoardRowMetrics.avatarSize)
                    }
                }
            }
            .monacoBleed()
            .task { await loader.load() }
        }
    }
}
#endif
