import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalRuleProposalCopy {
    static let title = "Propose a rule change"
    static let propose = "Propose"
    static let proposing = "Proposing\u{2026}"
    static let reasonPlaceholder = "Why? (optional)"
}

struct CabalRuleProposalSheet: View {
    let model: CabalRuleProposalModel
    let onFinish: (CabalRuleProposalModel.Outcome) -> Void

    var body: some View {
        @Bindable var model = model
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            words
                .font(MonacoTheme.Typo.rowTitle)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("cabal-rule-proposal-words")
            picker
            MonacoTextField(
                CabalRuleProposalCopy.reasonPlaceholder, text: $model.thesis, isMultiline: true
            )
            .accessibilityIdentifier("cabal-rule-proposal-reason")
            Text(model.passLine)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("cabal-rule-proposal-pass-line")
            if let refusal = model.refusal, refusal != .unchanged {
                Text(refusal.message)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.loss)
                    .accessibilityIdentifier("cabal-rule-proposal-refusal")
            }
            Button {
                Task { onFinish(await model.propose()) }
            } label: {
                SubmitLabel(
                    isWorking: model.isSubmitting, idle: CabalRuleProposalCopy.propose,
                    working: CabalRuleProposalCopy.proposing)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(!model.canSubmit)
            .accessibilityIdentifier("cabal-rule-proposal-propose")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.l)
        .monacoFittedSheet(title: CabalRuleProposalCopy.title)
    }

    private func name(_ userID: String) -> String {
        guard let member = model.cabal.members.first(where: { $0.userId == userID }) else { return "them" }
        return member.displayName.isEmpty ? (member.handle ?? "them") : member.displayName
    }

    @ViewBuilder private var words: some View {
        switch model.change {
        case .showSides(let on):
            Text("Show who voted what after a vote closes: **\(on ? "on" : "off")**")
        case .threshold(let value):
            Text("Votes to pass: **\(value.label)**")
        case .window(let value):
            Text("Vote window: **\(value.label)**")
        case .maxTrade(let bps):
            Text("Max per trade: **\(maxTradeLabel(bps))**")
        case .voter(let userID, let canVote):
            canVote ? Text("Make **\(name(userID))** a voter") : Text("Stop **\(name(userID))** voting")
        case .admin(let userID, let isAdmin):
            isAdmin ? Text("Make **\(name(userID))** an admin") : Text("Remove **\(name(userID))** as admin")
        }
    }

    private func maxTradeLabel(_ bps: Int32?) -> String {
        bps.map { "\($0 / 100)% of cash" } ?? "No limit"
    }

    @ViewBuilder private var picker: some View {
        switch model.change {
        case .threshold:
            Picker("Votes to pass", selection: thresholdBinding) {
                ForEach(CabalThreshold.allCases) { Text($0.label).tag($0) }
            }
            .pickerStyle(.segmented)
            .accessibilityIdentifier("cabal-rule-proposal-threshold")
        case .window:
            Picker("Vote window", selection: windowBinding) {
                ForEach(CabalProposalExpiry.allCases) { Text($0.label).tag($0) }
            }
            .pickerStyle(.segmented)
            .accessibilityIdentifier("cabal-rule-proposal-window")
        case .maxTrade:
            Picker("Max per trade", selection: maxTradeBinding) {
                ForEach(CabalRuleProposalModel.maxTradeChoices, id: \.self) { Text(maxTradeLabel($0)).tag($0) }
            }
            .pickerStyle(.menu)
            .accessibilityIdentifier("cabal-rule-proposal-max-trade")
        case .showSides, .voter, .admin:
            EmptyView()
        }
    }

    private var thresholdBinding: Binding<CabalThreshold> {
        Binding(
            get: {
                if case .threshold(let value) = model.change { return value }
                return .majority
            },
            set: { model.change = .threshold($0) })
    }

    private var windowBinding: Binding<CabalProposalExpiry> {
        Binding(
            get: {
                if case .window(let value) = model.change { return value }
                return .oneDay
            },
            set: { model.change = .window($0) })
    }

    private var maxTradeBinding: Binding<Int32?> {
        Binding(
            get: {
                if case .maxTrade(let value) = model.change { return value }
                return nil
            },
            set: { model.change = .maxTrade($0) })
    }
}
