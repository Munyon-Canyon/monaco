import MonacoCore
import SwiftUI

struct ProposalCard: View {
    private let summary: ProposalSummary
    let asset: ProposalAsset?
    let members: [ProposalMember]
    private let paused: Bool
    private let openRoute: AnyAppRoute?
    private let vote: (String) -> Void

    @State private var changing = false

    init(
        proposal: ProposalSummary, asset: ProposalAsset?, members: [ProposalMember], paused: Bool = false,
        openRoute: AnyAppRoute? = nil,
        vote: @escaping (String) -> Void = { _ in }
    ) {
        summary = proposal
        self.asset = asset
        self.members = members
        self.paused = paused
        self.openRoute = openRoute
        self.vote = vote
    }

    init(
        proposal: ProposalDetail, asset: ProposalAsset?, members: [ProposalMember], paused: Bool = false,
        openRoute: AnyAppRoute? = nil,
        vote: @escaping (String) -> Void = { _ in }
    ) {
        summary = proposal.summary
        self.asset = asset
        self.members = members
        self.paused = paused
        self.openRoute = openRoute
        self.vote = vote
    }

    private var isSell: Bool { summary.kind == "sell" }
    private var voter: ProposalMember? { members.first { $0.id == summary.proposerID } }
    private var canVote: Bool { summary.status == .open && summary.canVote }
    private var ballot: String? { summary.myBallot }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            header
            amount
            proposer
            if let thesis = summary.thesis, !thesis.isEmpty {
                Text(thesis)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.muted)
                    .lineLimit(3)
            }
            tracker
            if paused && summary.status == .open {
                Text(ProposalCardCopy.pausedCaption)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityIdentifier("proposal-paused-caption")
            }
            actions
        }
        .padding(MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MonacoTheme.surface, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
        .accessibilityIdentifier("proposal-card-\(summary.id)")
    }

    @ViewBuilder private var header: some View {
        if let openRoute {
            NavigationLink(value: openRoute) { headerRow }.buttonStyle(.plain)
        } else {
            headerRow
        }
    }

    private var headerRow: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: summary.symbol,
                displayName: asset?.displayName,
                assetKind: asset?.kind ?? .stock,
                size: 40,
                logoURL: asset?.logoURL
            )
            VStack(alignment: .leading, spacing: 2) {
                Text(
                    AssetDisplayName.format(
                        catalogName: asset?.displayName ?? summary.symbol, kind: asset?.kind ?? .stock)
                )
                .font(MonacoTheme.Typo.rowTitle)
                Text(isSell ? "Sell" : "Buy")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
            Spacer()
            if summary.status == .open {
                Text(ProposalCardCopy.closes(at: summary.expiresAt, now: .now))
                    .font(MonacoTheme.Typo.stamp)
                    .foregroundStyle(MonacoTheme.tertiaryText)
                    .accessibilityIdentifier("proposal-closes-in")
            } else if let label = ProposalChip.label(
                status: summary.status, isSell: isSell, swapFailed: summary.swap?.status == "failed"
            ) {
                Text(label)
                    .font(MonacoTheme.Typo.micro)
                    .foregroundStyle(summary.status == .passed ? MonacoTheme.ink : MonacoTheme.muted)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 5)
                    .background(Capsule().fill(MonacoTheme.surfaceSunken))
            }
        }
    }

    @ViewBuilder private var amount: some View {
        if isSell {
            Text(shareLabel)
                .moneyFont(.large)
                .foregroundStyle(MonacoTheme.ink)
        } else if let micros = summary.usdcMicros {
            MoneyText(micros: micros, style: .large)
        }
    }

    private var proposer: some View {
        NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: summary.proposerID))) {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(
                    photoURL: voter?.photoURL?.absoluteString, displayName: voter?.name ?? "Member", size: 28,
                    seed: summary.proposerID)
                Text("\(voter?.name ?? "Member") · \(ageLabel)")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
        }
        .buttonStyle(.plain)
    }

    private var tracker: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            HStack(spacing: 4) {
                ForEach(0..<summary.tally.voters, id: \.self) { index in
                    Circle().fill(index < summary.tally.yes + summary.tally.no ? MonacoTheme.ink : MonacoTheme.hairline)
                        .frame(width: 8, height: 8)
                }
            }
            Text(
                "\(summary.tally.yes + summary.tally.no) of \(summary.tally.voters) voted · \(summary.tally.needed) yes to pass"
            )
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
        }
    }

    @ViewBuilder private var actions: some View {
        if canVote {
            if let ballot, !changing {
                HStack {
                    Text("✓ You voted \(ballot)").font(MonacoTheme.Typo.calloutStrong)
                    Spacer()
                    Button("Change") { changing = true }.buttonStyle(.monacoSecondary)
                }
            } else {
                HStack(spacing: MonacoTheme.Space.s) {
                    Button("Yes") {
                        vote("yes")
                        changing = false
                    }.buttonStyle(.monacoPrimary)
                    Button("No") {
                        vote("no")
                        changing = false
                    }.buttonStyle(.monacoSecondary)
                }
            }
        }
    }

    private var ageLabel: String {
        let minutes = max(0, Int(Date.now.timeIntervalSince(summary.createdAt) / 60))
        return minutes < 60 ? "\(minutes)m" : "\(minutes / 60)h"
    }

    private var shareLabel: String {
        guard let amount = summary.tokenAmount else { return "0 shares" }
        guard asset?.kind != .preIpo else { return "\(amount) tokens" }
        let decimals = asset?.decimals ?? AssetCatalogDefaults.decimals
        let digits = String(amount)
        let padded = String(repeating: "0", count: max(0, decimals + 1 - digits.count)) + digits
        let split = padded.index(padded.endIndex, offsetBy: -decimals)
        let whole = String(padded[..<split])
        let fraction = String(padded[split...]).prefix(4).reversed().drop(while: { $0 == "0" }).reversed()
        return fraction.isEmpty ? "\(whole) shares" : "\(whole).\(fraction) shares"
    }
}
