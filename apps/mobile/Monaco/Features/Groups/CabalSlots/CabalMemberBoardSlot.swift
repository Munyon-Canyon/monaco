import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalMemberBoardSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalMemberBoard(cabalID: context.cabalID)
            .id("cabal-member-board")
    }
}

enum CabalVoterBoardSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalMemberBoard(cabalID: context.cabalID, votersOnly: true)
            .id("cabal-voter-board")
    }
}

private struct CabalMemberBoard: View {
    let cabalID: String
    var votersOnly = false
    @Environment(\.cabalRetry) private var retry
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.cabalPotModel) private var potModel

    private var hasNoPositions: Bool {
        guard let state = potModel?.summary?.state else { return false }
        return state != .invested
    }

    var body: some View {
        if hasNoPositions {
            Color.clear.frame(height: 0)
        } else {
            board
        }
    }

    private var voterIDs: Set<String> {
        Set((cabalModel?.cabal?.members ?? []).filter(\.canVote).map(\.userId))
    }

    private var board: some View {
        let voters = voterIDs
        return LeaderboardHost(
            board: .cabalMembers(id: cabalID), refreshKey: "cabal-member-board", reloadID: retry.tick
        ) { loader in
            let lastVisible = votersOnly ? loader.rows.last(where: { voters.contains($0.id) })?.id : nil
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader(votersOnly ? "Voter standings" : "Member standings")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.bottom, -MonacoTheme.Space.s)
                Text(helper)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .accessibilityIdentifier("cabal-member-board-helper")
                LeaderboardBoardList(
                    loader: loader, skeletonRows: 3, failureThing: "the standings",
                    identifier: "cabal-member-board", showsEmpty: false,
                    empty: { EmptyView() },
                    rowContent: { row, isLast in
                        if !votersOnly || voters.contains(row.id) {
                            NavigationLink(
                                value: AnyAppRoute(
                                    UserProfileRoute(
                                        userID: row.id,
                                        preview: UserPreview(
                                            displayName: row.name, handle: row.handle, photoURL: row.pictureURL)
                                    ))
                            ) {
                                CabalStandingRow(row: row, isLast: votersOnly ? row.id == lastVisible : isLast)
                            }
                            .buttonStyle(.monacoRow)
                            .accessibilityIdentifier("cabal-member-\(row.id)")
                        }
                    })
                LeaderboardFreshnessText(
                    loader: loader, identifier: "cabal-member-board-freshness", font: MonacoTheme.Typo.caption
                )
                .padding(.horizontal, MonacoTheme.Space.gutter)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .accessibilityElement(children: .contain)
        }
    }

    private var helper: String {
        let name = cabalModel?.cabal?.name ?? "this cabal"
        return "Within \(name) \u{00B7} Share return \u{00B7} \(LeaderboardRange.oneMonth.label)"
    }
}

private struct CabalStandingRow: View {
    let row: LeaderboardRowView
    let isLast: Bool

    private static let rankWidth: CGFloat = 18

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Text("\(row.rank)")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(minWidth: Self.rankWidth)
                .accessibilityHidden(true)
            MonacoAvatar(photoURL: row.pictureURL, displayName: row.name, size: .feed, seed: row.id)
            Text(row.name)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(1)
                .truncationMode(.tail)
                .monacoPrivate()
            if row.isViewer {
                StatusBadge(BoardRow<EmptyView>.viewerLabel, tone: .positive)
            }
            Spacer(minLength: MonacoTheme.Space.s)
            figure
        }
        .padding(.horizontal, MonacoTheme.Space.sm)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .background {
            if row.isViewer {
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous)
                    .fill(MonacoTheme.brandSoft)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.s)
        .overlay(alignment: .bottom) {
            if !isLast, !row.isViewer { MonacoRule().padding(.horizontal, MonacoTheme.Space.gutter) }
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityLabel(BoardRow<EmptyView>.spoken(row, figure: .pnl))
    }

    @ViewBuilder private var figure: some View {
        if let bps = row.returnBps {
            PercentText(basisPoints: bps, style: .row)
                .lineLimit(1)
        } else {
            Text("\u{2014}")
                .moneyFont(.row)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
    }
}
