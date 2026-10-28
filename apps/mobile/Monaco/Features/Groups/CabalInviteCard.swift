import MonacoCore
import SwiftUI

enum CabalInviteMetrics {
    static let coverHeight: CGFloat = 48
    static let crestSize: CGFloat = 56
    static let inviterPhoto: CGFloat = 32
    static let faceSize: CGFloat = 32
    static let faceOverlap: CGFloat = 10
    static let proposalLogo: CGFloat = 24
    static let qrTile: CGFloat = 200
    static let skeletonLink: CGFloat = 220
    static let skeletonCard: CGFloat = 232
    static let flipDuration: Double = 0.35
    static let shareWidth: CGFloat = 358
    static let shareScale: CGFloat = 3
    static let roundDiameter: CGFloat = 32
    static let roundGap: CGFloat = 8
}

struct CabalInviteCard: View {
    let preview: CabalInvitePreview

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            cover
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                identity
                inviter
                if let proposal = preview.proposal {
                    Rectangle()
                        .fill(MonacoTheme.hairline)
                        .frame(height: 1)
                    CabalInviteProposalRow(proposal: proposal)
                }
            }
            .padding(MonacoTheme.Space.m)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(MonacoTheme.surfaceSunken)
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-invite-link")
    }

    private var cover: some View {
        CabalCover(
            cabalId: preview.cabalID, tint: .forGroupId(preview.cabalID),
            pictureURL: CabalMark.resolvedURL(preview.pictureURL), style: .card
        ) {
            Color.clear
        }
        .frame(height: CabalInviteMetrics.coverHeight)
        .accessibilityElement(children: .ignore)
        .accessibilityAddTraits(.isImage)
        .accessibilityLabel("\(preview.name) cover")
        .accessibilityIdentifier("cabal-invite-cover")
    }

    private var identity: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            CabalMark(
                groupId: preview.cabalID, name: preview.name, size: CabalInviteMetrics.crestSize,
                pictureUrl: preview.pictureURL)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(preview.name)
                    .font(MonacoTheme.Typo.title2)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? 3 : 1)
                Text(preview.facts)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("cabal-invite-facts")
            }
        }
    }

    @ViewBuilder private var inviter: some View {
        if let person = preview.inviter, let line = preview.inviterLine {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(
                    photoURL: person.photoURL, displayName: person.displayName, size: CabalInviteMetrics.inviterPhoto)
                Text(line)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                    .accessibilityIdentifier("cabal-invite-inviter")
                Spacer(minLength: MonacoTheme.Space.s)
                CabalInviteFaces(faces: preview.faces, moreText: preview.moreText)
            }
        }
    }
}

private struct CabalInviteFaces: View {
    let faces: [CabalInvitePreview.Person]
    let moreText: String?

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            HStack(spacing: -CabalInviteMetrics.faceOverlap) {
                ForEach(Array(faces.enumerated()), id: \.offset) { index, person in
                    MonacoAvatar(
                        photoURL: person.photoURL, displayName: person.displayName,
                        size: CabalInviteMetrics.faceSize, ring: .surface
                    )
                    .zIndex(Double(faces.count - index))
                }
            }
            if let moreText {
                Text(moreText)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        .accessibilityHidden(true)
    }
}

private struct CabalInviteProposalRow: View {
    let proposal: CabalInvitePreview.Proposal

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
            StockMark(
                symbol: proposal.symbol, displayName: proposal.assetName, size: CabalInviteMetrics.proposalLogo,
                logoURL: CabalMark.resolvedURL(proposal.logoURL))
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(proposal.title)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                Text(proposal.detail)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? 3 : 2)
            }
            Spacer(minLength: MonacoTheme.Space.s)
            counts
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            "\(proposal.title). \(proposal.detail). \(proposal.yes) yes, \(proposal.no) no"
        )
        .accessibilityIdentifier("cabal-invite-proposal")
    }

    private var counts: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Label("\(proposal.yes)", systemImage: "checkmark")
                .foregroundStyle(MonacoTheme.profit)
            Label("\(proposal.no)", systemImage: "xmark")
                .foregroundStyle(MonacoTheme.loss)
        }
        .font(MonacoTheme.Typo.headline)
        .fixedSize()
    }
}
