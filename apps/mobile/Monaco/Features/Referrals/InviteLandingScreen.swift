import MonacoAPI
import MonacoCore
import SwiftUI

private enum InviteLandingMetrics {
    static let coverHeight: CGFloat = 120
    static let crest: CGFloat = 56
    static let photo: CGFloat = 32
    static let face: CGFloat = 32
    static let faceOverlap: CGFloat = 10
    static let logo: CGFloat = 32
    static let round: CGFloat = 32
}

struct InviteLandingScreen: View {
    let model: InviteLandingModel
    let shareURL: URL
    let onRedirect: (InviteLandingModel.Redirect) -> Void

    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        content
            .monacoCanvas()
            .toolbar {
                ToolbarItem(placement: .principal) {
                    MonacoWordmark(.toolbar, isTitle: true)
                        .fixedSize()
                }
                .withoutGlass()
                ToolbarItem(placement: .topBarTrailing) {
                    ShareLink(item: shareURL) {
                        Image(systemName: "square.and.arrow.up")
                            .font(MonacoTheme.Typo.headline)
                            .foregroundStyle(MonacoTheme.primaryText)
                            .frame(minWidth: 44, minHeight: 44)
                    }
                    .accessibilityLabel("Share invite link")
                    .accessibilityIdentifier("invite-landing-share")
                }
            }
            .toolbarTitleDisplayMode(.inline)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("invite-landing")
            .onChange(of: model.redirect, initial: true) { _, target in
                if let target { onRedirect(target) }
            }
            .onChange(of: model.toast) { _, message in
                guard let message else { return }
                toasts.current = MonacoToast(message: message, isSuccess: message.hasPrefix("Request sent"))
                model.toast = nil
            }
    }

    @ViewBuilder private var content: some View {
        switch model.phase {
        case .loading:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                SkeletonBlock(height: InviteLandingMetrics.coverHeight, radius: MonacoTheme.Radius.card)
                SkeletonBlock(width: 200, height: 24)
                SkeletonBlock(width: 140, height: 16)
                SkeletonBlock(height: 72, radius: MonacoTheme.Radius.card)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Loading")
            .accessibilityIdentifier("invite-landing-loading")
        case .failed:
            EmptyState(
                title: "Couldn\u{2019}t load this invite", message: "Check your connection and try again.",
                actionTitle: "Try again", action: { Task { await model.load() } }
            )
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .accessibilityIdentifier("invite-landing-failed")
        case .loaded:
            if let landing = model.content {
                ScrollView {
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                        cover(landing)
                        identity(landing)
                        inviter(landing)
                        members(landing)
                        stats(landing.stats)
                        if let proposal = landing.proposal {
                            InviteLandingProposal(
                                proposal: proposal, eligible: Int(model.preview?.openProposal?.eligible ?? 0))
                        }
                        joinControls
                    }
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.s)
                    .padding(.bottom, MonacoTheme.Space.l)
                }
                .scrollBounceBehavior(.basedOnSize)
            } else {
                Color.clear
            }
        }
    }

    private func cover(_ landing: InviteLandingContent) -> some View {
        let id = model.cabalID
        return CabalCover(
            cabalId: id, tint: .forGroupId(id), pictureURL: CabalMark.resolvedURL(landing.pictureURL), style: .card
        ) {
            Color.clear
        }
        .frame(height: InviteLandingMetrics.coverHeight)
        .accessibilityElement(children: .ignore)
        .accessibilityAddTraits(.isImage)
        .accessibilityLabel("\(landing.name) cover")
        .accessibilityIdentifier("invite-landing-cover")
    }

    private func identity(_ landing: InviteLandingContent) -> some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            CabalMark(
                groupId: model.cabalID, name: landing.name, size: InviteLandingMetrics.crest,
                pictureUrl: landing.pictureURL)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(landing.name)
                    .font(MonacoTheme.Typo.title2)
                    .foregroundStyle(MonacoTheme.primaryText)
                Text(landing.facts)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("invite-landing-identity")
    }

    @ViewBuilder private func inviter(_ landing: InviteLandingContent) -> some View {
        if let person = landing.inviter, let line = landing.inviterLine {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(
                    photoURL: person.photoURL, displayName: person.displayName, size: InviteLandingMetrics.photo)
                Text(line)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.primaryText)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("invite-landing-inviter")
        }
    }

    @ViewBuilder private func members(_ landing: InviteLandingContent) -> some View {
        if let line = landing.membersLine {
            HStack(spacing: MonacoTheme.Space.s) {
                HStack(spacing: -InviteLandingMetrics.faceOverlap) {
                    ForEach(Array(landing.faces.enumerated()), id: \.offset) { index, person in
                        MonacoAvatar(
                            photoURL: person.photoURL, displayName: person.displayName,
                            size: InviteLandingMetrics.face, ring: .surface
                        )
                        .zIndex(Double(landing.faces.count - index))
                    }
                }
                .accessibilityHidden(true)
                Text(line)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("invite-landing-members")
        }
    }

    private func stats(_ stats: InviteLandingContent.Stats) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.sm))
            : AnyLayout(HStackLayout(alignment: .top, spacing: MonacoTheme.Space.s))
        return layout {
            figure("Treasury", stats.treasury)
            figure("1M", stats.oneMonth, color: stats.oneMonthIsLoss ? MonacoTheme.loss : MonacoTheme.primaryText)
            figure("Turnout", stats.turnout)
        }
        .padding(MonacoTheme.Space.m)
        .background(MonacoTheme.surfaceSunken)
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("invite-landing-stats")
    }

    private func figure(_ label: String, _ value: String, color: Color = MonacoTheme.primaryText) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            Text(value)
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(color)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var joinControls: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            if model.showsAcceptDecline {
                HStack(spacing: MonacoTheme.Space.s) { buttons }
            } else {
                buttons
            }
            if let note = model.note {
                Text(note)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                    .frame(maxWidth: .infinity)
                    .accessibilityIdentifier("invite-landing-note")
            }
        }
        .frame(maxWidth: .infinity)
    }

    @ViewBuilder private var buttons: some View {
        if model.showsAcceptDecline {
            Button("Decline") { Task { await model.decline() } }
                .buttonStyle(.monacoOutline)
                .accessibilityIdentifier("invite-landing-decline")
            Button("Accept") { Task { await model.accept() } }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("invite-landing-accept")
        } else {
            Button(model.buttonTitle) { Task { await model.join() } }
                .buttonStyle(.monacoPrimary)
                .disabled(!model.buttonEnabled)
                .accessibilityIdentifier("invite-landing-join")
        }
    }
}

private struct InviteLandingProposal: View {
    let proposal: InviteLandingContent.Proposal
    let eligible: Int

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            Text("Open now")
                .font(MonacoTheme.Typo.section)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityAddTraits(.isHeader)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    StockMark(
                        symbol: proposal.symbol, displayName: proposal.assetName, size: InviteLandingMetrics.logo,
                        logoURL: CabalMark.resolvedURL(proposal.logoURL))
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                        Text(proposal.title)
                            .font(MonacoTheme.Typo.headline)
                            .foregroundStyle(MonacoTheme.primaryText)
                        Text(proposal.byline)
                            .font(MonacoTheme.Typo.subhead)
                            .foregroundStyle(MonacoTheme.secondaryText)
                            .lineLimit(dynamicTypeSize.isAccessibilitySize ? 3 : 1)
                    }
                }
                VoteTallyBar(yes: proposal.yes, no: proposal.no, eligible: max(eligible, proposal.yes + proposal.no))
                VoteTallyEnds(
                    yes: proposal.yes, no: proposal.no, middle: proposal.voteNote, identifierPrefix: "invite-landing")
                Text(proposal.detail)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
            .padding(MonacoTheme.Space.m)
            .background(MonacoTheme.surfaceSunken)
            .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("invite-landing-proposal")
        }
    }
}

extension ToolbarContent {
    @ToolbarContentBuilder
    fileprivate func withoutGlass() -> some ToolbarContent {
        if #available(iOS 26.0, *) {
            sharedBackgroundVisibility(.hidden)
        } else {
            self
        }
    }
}
