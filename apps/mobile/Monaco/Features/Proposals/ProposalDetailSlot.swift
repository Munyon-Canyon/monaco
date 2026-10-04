import MonacoAPI
import MonacoCore
import SwiftUI

enum ProposalDetailSlot: ProposalSection {
    static let isLive = true

    static func body(for context: ProposalContext) -> some View {
        ProposalDetailLive(proposalID: context.proposalID)
    }
}

private struct ProposalDetailLive: View {
    let proposalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: ProposalDetailModel?

    var body: some View {
        Group {
            if let model {
                ProposalDetailSection(model: model)
            } else {
                ProposalDetailSkeleton()
            }
        }
        .task {
            guard model == nil else { return }
            model = ProposalDetailModel(
                proposalID: proposalID, repository: ProposalsRepository(api: environment.api), hints: environment.hints)
        }
    }
}

struct ProposalDetailSection: View {
    let model: ProposalDetailModel

    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .overlay(alignment: .top) {
                ProposalCoinBurst(trigger: model.celebrations)
                    .padding(.top, 120)
            }
            .task {
                refresh?.register("proposal-detail") { await model.load() }
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { model.setVisible($0) }
            .onChange(of: model.voting.toast) { _, toast in
                guard let toast else { return }
                toasts.current = MonacoToast(message: toast.message, isSuccess: toast.isSuccess)
            }
    }

    @ViewBuilder
    private var content: some View {
        switch model.state {
        case .idle, .loading:
            ProposalDetailSkeleton()
        case .failed:
            ProposalLoadFailedRow(message: "Couldn't load this proposal.") {
                Task { await model.load() }
            }
        case .loaded(let screen):
            ProposalDetailContent(
                screen: screen,
                isCasting: model.voting.casting.contains(screen.card.id),
                vote: { choice in Task { await model.vote(choice) } }
            )
        }
    }
}

struct ProposalDetailContent: View {
    let screen: ProposalPage
    let isCasting: Bool
    let vote: (BallotChoice) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            TimelineView(.everyMinute) { context in
                VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                    ProposalCardHeader(card: screen.card, now: context.date)
                    let proposal = screen.card.proposal
                    ProposalByline(
                        person: screen.card.proposer,
                        text: ProposalCardCopy.proposedBy(
                            screen.card.proposer.name, since: proposal.createdAt, now: context.date))
                }
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                ProposalTracker(card: screen.card)
                ForEach(screen.voters) { voter in
                    ProposalVoterRow(line: voter)
                }
            }
            ProposalBallotRow(ballot: screen.card.ballot, isCasting: isCasting, vote: vote)
            if let thesis = screen.card.proposal.thesis {
                section(screen.reasonTitle) {
                    HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                        Capsule()
                            .fill(MonacoTheme.hairline)
                            .frame(width: 3)
                        Text(thesis)
                            .font(MonacoTheme.Typo.body)
                            .foregroundStyle(MonacoTheme.ink)
                            .fixedSize(horizontal: false, vertical: true)
                            .accessibilityIdentifier("proposal-reason")
                    }
                }
            }
            section("Expected") {
                Text(screen.expected)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.ink)
                    .accessibilityIdentifier("proposal-expected")
            }
            section("Status") {
                ProposalStepper(steps: screen.steps, votingSince: screen.card.proposal.createdAt)
            }
            if let tradeID = screen.tradeID {
                NavigationLink(
                    value: AnyAppRoute(
                        TransactionRoute(cabalID: screen.card.proposal.cabalID, transactionID: tradeID))
                ) {
                    HStack {
                        Text("View transaction")
                            .font(MonacoTheme.Typo.bodyStrong)
                            .foregroundStyle(MonacoTheme.brand)
                        Spacer()
                        Image(systemName: "chevron.right")
                            .foregroundStyle(MonacoTheme.muted)
                            .accessibilityHidden(true)
                    }
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("proposal-view-transaction")
            }
        }
        .padding(.vertical, MonacoTheme.Space.m)
    }

    private func section(_ title: String, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(title)
            content()
        }
    }
}

private struct ProposalVoterRow: View {
    let line: ProposalVoterLine

    var body: some View {
        NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: line.person.userID))) {
            HStack(spacing: MonacoTheme.Space.s) {
                MonacoAvatar(
                    photoURL: line.person.photoURL, displayName: line.person.name, size: 28, seed: line.person.userID)
                Text(line.text)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.ink)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 0)
            }
            .frame(minHeight: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("proposal-voter-line")
    }
}

private struct ProposalStepper: View {
    let steps: ProposalSteps
    let votingSince: Date

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            ForEach(Array(steps.titles.enumerated()), id: \.offset) { index, title in
                HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                    marker(index)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(title)
                            .font(index == steps.reached ? MonacoTheme.Typo.bodyStrong : MonacoTheme.Typo.body)
                            .foregroundStyle(index <= steps.reached ? MonacoTheme.ink : MonacoTheme.muted)
                        if index == 0 {
                            Text(votingSince, format: .dateTime.month(.abbreviated).day().hour().minute())
                                .font(MonacoTheme.Typo.caption)
                                .foregroundStyle(MonacoTheme.muted)
                        }
                        if index == steps.titles.count - 1, steps.failed, let message = steps.failureMessage {
                            Text(message)
                                .font(MonacoTheme.Typo.caption)
                                .foregroundStyle(MonacoTheme.loss)
                                .fixedSize(horizontal: false, vertical: true)
                                .accessibilityIdentifier("proposal-failure-message")
                        }
                    }
                }
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("proposal-step-\(index)")
            }
        }
    }

    private func marker(_ index: Int) -> some View {
        let reached = index <= steps.reached
        let failed = steps.failed && index == steps.titles.count - 1
        return Circle()
            .fill(reached ? (failed ? MonacoTheme.loss : MonacoTheme.brandFill) : Color.clear)
            .overlay(Circle().strokeBorder(reached ? Color.clear : MonacoTheme.hairline, lineWidth: 1.5))
            .frame(width: 12, height: 12)
            .padding(.top, 5)
    }
}

struct ProposalDetailSkeleton: View {
    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            HStack(spacing: MonacoTheme.Space.sm) {
                SkeletonBlock(width: 40, height: 40, radius: 20)
                VStack(alignment: .leading, spacing: 6) {
                    SkeletonBlock(width: 64, height: 14)
                    SkeletonBlock(width: 36, height: 10)
                }
                Spacer()
            }
            SkeletonBlock(width: 140, height: 28)
            SkeletonBlock(width: 180, height: 14)
            HStack(spacing: 6) {
                ForEach(0..<3, id: \.self) { _ in SkeletonBlock(width: 10, height: 10, radius: 5) }
            }
            SkeletonBlock(width: 160, height: 10)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading")
        .accessibilityIdentifier("proposal-detail-skeleton")
    }
}

#if DEBUG
final class ProposalDetailSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-proposalDetailHarness"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: launchArgument) else { return nil }
        let scenario = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "open"
        return AnyView(ProposalDetailHarnessScreen(scenario: scenario))
    }
}

private struct ProposalDetailHarnessScreen: View {
    let scenario: String

    @State private var model: ProposalDetailModel?
    @State private var refresh = ScreenRefresh()

    var body: some View {
        NavigationStack {
            ScrollView {
                if let model {
                    ProposalDetailSection(model: model)
                }
            }
            .monacoCanvas()
            .environment(refresh)
            .refreshable { await refresh.run() }
            .navigationBarTitleDisplayMode(.inline)
            .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .task {
            guard model == nil else { return }
            let server = Self.server(scenario)
            if scenario == "error" { await server.setFailing(true) }
            model = ProposalDetailModel(
                proposalID: Components.Schemas.ProposalDetail.sample(now: Date()).id,
                repository: .preview(server), hints: ProposalsPreviewHints())
        }
    }

    private static func server(_ scenario: String) -> ProposalsPreviewServer {
        let now = Date()
        let viewer = Components.Schemas.ProposalDetail.sampleVoterIDs[0]
        let priya = Components.Schemas.ProposalDetail.sampleVoterIDs[2]
        let proposal: Components.Schemas.ProposalDetail =
            switch scenario {
            case "voted": .sample(ballots: [viewer: .yes, priya: .no], now: now)
            case "sell": .sample(kind: .sell, now: now)
            case "nonvoter": .sample(canVote: false, now: now)
            case "blocked":
                .sample(
                    status: .executionBlocked,
                    statusMessage: "The pot doesn't have enough cash for this buy.", now: now)
            case "failed": .sample(status: .passed, swap: .failed(retryable: true), now: now)
            case "failedFinal": .sample(status: .passed, swap: .failed(retryable: false), now: now)
            case "executed": .sample(status: .executed, ballots: [viewer: .yes, priya: .yes], now: now)
            default: .sample(now: now)
            }
        return ProposalsPreviewServer(proposals: [proposal])
    }
}
#endif
