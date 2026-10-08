import MonacoCore
import SwiftUI

struct CabalRulesSection: View {
    @Binding var joinPolicy: CabalJoinPolicy
    @Binding var voterSet: CabalVoterMode
    @Binding var threshold: CabalThreshold
    @Binding var voteExpiry: CabalProposalExpiry
    let identifierPrefix: String

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(CabalRulesCopy.sectionTitle)
                .padding(.horizontal, MonacoTheme.Space.gutter)

            MonacoGroupedList {
                CabalRuleRow(
                    title: CabalRulesCopy.joinTitle,
                    options: CabalJoinPolicy.allCases,
                    selection: $joinPolicy,
                    label: { $0.label },
                    caption: { $0.caption },
                    identifier: "\(identifierPrefix)-join"
                )
                CabalRuleRow(
                    title: CabalRulesCopy.votersTitle,
                    options: CabalVoterMode.allCases,
                    selection: $voterSet,
                    label: { $0.label },
                    caption: { $0.caption },
                    identifier: "\(identifierPrefix)-voters"
                )
                CabalRuleRow(
                    title: CabalRulesCopy.thresholdTitle,
                    options: CabalThreshold.allCases,
                    selection: $threshold,
                    label: { $0.label },
                    caption: { $0.caption },
                    identifier: "\(identifierPrefix)-threshold"
                )
                CabalRuleRow(
                    title: CabalRulesCopy.expiryTitle,
                    options: CabalProposalExpiry.allCases,
                    selection: $voteExpiry,
                    label: { $0.label },
                    caption: { $0.caption },
                    identifier: "\(identifierPrefix)-expiry",
                    isLast: true
                )
            }
        }
    }
}

private struct CabalRuleRow<Option: Hashable>: View {
    let title: String
    let options: [Option]
    @Binding var selection: Option
    let label: (Option) -> String
    let caption: (Option) -> String
    let identifier: String
    var isLast = false

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(title)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.ink)
                    .accessibilityAddTraits(.isHeader)
                Text(caption(selection))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
                    .contentTransition(.opacity)
                    .animation(reduceMotion ? nil : .easeOut(duration: 0.2), value: selection)
            }
            CabalRuleChoice(options: options, selection: $selection, label: label)
        }
        .padding(MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule()
                    .padding(.leading, MonacoTheme.Space.m)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(identifier)
    }
}

private struct CabalRuleChoice<Option: Hashable>: View {
    let options: [Option]
    @Binding var selection: Option
    let label: (Option) -> String

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(spacing: MonacoTheme.Space.s) {
                ForEach(options, id: \.self) { option in
                    stackedOption(option)
                }
            }
        } else {
            MonacoSegmented(options, selection: $selection, label: label)
        }
    }

    private func stackedOption(_ option: Option) -> some View {
        let isSelected = option == selection
        return Button {
            guard !isSelected else { return }
            Haptics.selection()
            selection = option
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(label(option))
                    .font(MonacoTheme.Typo.calloutStrong)
                    .multilineTextAlignment(.leading)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if isSelected {
                    Image(systemName: "checkmark")
                        .font(MonacoTheme.Typo.calloutStrong)
                        .accessibilityHidden(true)
                }
            }
            .foregroundStyle(isSelected ? MonacoTheme.onBrand : MonacoTheme.ink)
            .padding(.horizontal, MonacoTheme.Space.m)
            .padding(.vertical, MonacoTheme.Space.s)
            .frame(minHeight: 44)
            .background(Capsule().fill(isSelected ? MonacoTheme.brandFill : MonacoTheme.surfaceSunken))
            .contentShape(Capsule())
        }
        .buttonStyle(.plain)
        .accessibilityAddTraits(isSelected ? [.isSelected] : [])
    }
}
