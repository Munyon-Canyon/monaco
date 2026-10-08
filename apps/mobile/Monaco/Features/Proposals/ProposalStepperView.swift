import MonacoCore
import SwiftUI

struct ProposalStepperView: View {
    let stepper: ProposalStepper
    @ScaledMetric(relativeTo: .callout) private var markerSize: CGFloat = 22

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(stepper.steps.enumerated()), id: \.offset) { index, step in
                stepRow(step, isLast: index == stepper.steps.count - 1)
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(stepper.accessibilityLabel)
        .accessibilityIdentifier("proposal-tracker")
    }

    private func stepRow(_ step: ProposalStepper.Step, isLast: Bool) -> some View {
        let marker = stepMarker(step.mark)
        return HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
            VStack(spacing: 0) {
                Image(systemName: marker.symbol)
                    .font(MonacoTheme.Typo.calloutStrong)
                    .foregroundStyle(marker.color)
                    .frame(width: markerSize, height: markerSize)
                if !isLast {
                    Rectangle()
                        .fill(step.mark == .done ? MonacoTheme.ink : MonacoTheme.hairline)
                        .frame(width: 2)
                        .frame(maxHeight: .infinity)
                }
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(step.title)
                    .font(step.mark == .pending ? MonacoTheme.Typo.callout : MonacoTheme.Typo.calloutStrong)
                    .foregroundStyle(titleColor(step.mark))
                if let stamp = step.stamp {
                    Text(stamp.text).font(MonacoTheme.Typo.stamp).foregroundStyle(MonacoTheme.tertiaryText)
                }
                if let note = step.note {
                    Text(note).font(MonacoTheme.Typo.callout).foregroundStyle(MonacoTheme.muted)
                }
            }
            .padding(.bottom, isLast ? 0 : MonacoTheme.Space.s)
            Spacer(minLength: 0)
        }
    }

    private func titleColor(_ mark: ProposalStepper.Mark) -> Color {
        switch mark {
        case .failed: MonacoTheme.loss
        case .pending: MonacoTheme.muted
        case .done, .active: MonacoTheme.ink
        }
    }

    private func stepMarker(_ mark: ProposalStepper.Mark) -> (symbol: String, color: Color) {
        switch mark {
        case .done: ("checkmark.circle.fill", MonacoTheme.ink)
        case .active: ("circle.inset.filled", MonacoTheme.brand)
        case .pending: ("circle", MonacoTheme.tertiaryText)
        case .failed: ("xmark.circle.fill", MonacoTheme.loss)
        }
    }
}
