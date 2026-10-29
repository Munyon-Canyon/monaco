import Foundation
import MonacoAPI
import Observation

public struct LedgerCall: Equatable, Sendable, Identifiable {
    public enum Kind: Equatable, Sendable { case buy, sell }

    public let proposalID: String
    public let cabalName: String
    public let kind: Kind
    public let symbol: String
    public let logoURL: String?
    public let votedYes: Bool
    public let moveBps: Int
    public let right: Bool

    public var id: String { proposalID }

    public init(
        proposalID: String, cabalName: String, kind: Kind, symbol: String, logoURL: String?, votedYes: Bool,
        moveBps: Int, right: Bool
    ) {
        self.proposalID = proposalID
        self.cabalName = cabalName
        self.kind = kind
        self.symbol = symbol
        self.logoURL = logoURL
        self.votedYes = votedYes
        self.moveBps = moveBps
        self.right = right
    }
}

public struct LedgerReply: Equatable, Sendable {
    public let proposalLabel: String
    public let authorName: String
    public let authorPhotoURL: String?
    public let excerpt: String
    public let topEmoji: String
    public let topEmojiCount: Int
}

public struct LedgerScreen: Equatable, Sendable {
    public let week: String
    public let title: String
    public let returnBps: Int64?
    public let cabalCount: Int
    public let voted: Int
    public let eligible: Int
    public let right: Int
    public let resolved: Int
    public let sharpestName: String?
    public let bestCall: LedgerCall?
    public let miss: LedgerCall?
    public let calls: [LedgerCall]
    public let topReply: LedgerReply?

    public init(_ ledger: Components.Schemas.Ledger) {
        week = ledger.week
        title = LedgerCopy.title(start: ledger.labelStart, end: ledger.labelEnd)
        returnBps = ledger.stakeReturnBps
        cabalCount = ledger.cabals.count
        voted = Int(ledger.voted)
        eligible = Int(ledger.eligible)
        right = Int(ledger.right)
        resolved = Int(ledger.resolved)
        sharpestName = ledger.sharpest?.user.name
        bestCall = ledger.bestCall.map {
            LedgerCall(
                proposalID: $0.proposalId, cabalName: $0.cabal.name, kind: $0.kind == .buy ? .buy : .sell,
                symbol: $0.symbol, logoURL: $0.assetLogoUrl, votedYes: $0.yourChoice == .yes,
                moveBps: Int($0.moveBps), right: $0.right)
        }
        miss = ledger.miss.map {
            LedgerCall(
                proposalID: $0.proposalId, cabalName: $0.cabal.name, kind: $0.kind == .buy ? .buy : .sell,
                symbol: $0.symbol, logoURL: $0.assetLogoUrl, votedYes: $0.yourChoice == .yes,
                moveBps: Int($0.moveBps), right: $0.right)
        }
        calls = ledger.calls.map {
            LedgerCall(
                proposalID: $0.proposalId, cabalName: $0.cabal.name, kind: $0.kind == .buy ? .buy : .sell,
                symbol: $0.symbol, logoURL: $0.assetLogoUrl, votedYes: $0.yourChoice == .yes,
                moveBps: Int($0.moveBps), right: $0.right)
        }
        topReply = ledger.topReply.map {
            LedgerReply(
                proposalLabel: $0.proposalLabel, authorName: $0.author.name, authorPhotoURL: $0.author.pictureUrl,
                excerpt: $0.excerpt, topEmoji: $0.topEmoji, topEmojiCount: Int($0.topEmojiCount))
        }
    }
}

@Observable
@MainActor
public final class LedgerModel {
    public private(set) var state: LoadState<LedgerScreen> = .idle

    private let api: APIClient
    private let week: String

    public init(api: APIClient, week: String) {
        self.api = api
        self.week = week
    }

    public func load() async {
        if case .loaded = state {} else { state = .loading }
        do {
            let week = week
            let ledger = try await api.read { client in
                try await client.getMyLedger(path: .init(week: week)).ok.body.json
            }
            state = .loaded(LedgerScreen(ledger))
        } catch {
            state = .failed(APIError(error))
        }
    }
}

#if DEBUG
extension LedgerModel {
    public static func preview(state: LoadState<LedgerScreen>, week: String = "2026-W42") -> LedgerModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        let model = LedgerModel(
            api: APIClient(serverURL: serverURL, tokens: PreviewTokens(), transport: PreviewTransport()), week: week)
        model.state = state
        return model
    }
}

private struct PreviewTransport: ClientTransport {
    func send(
        _ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        throw URLError(.notConnectedToInternet)
    }
}

private struct PreviewTokens: AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}
#endif
