import MonacoCore
import SwiftUI

struct OnboardingNudgeBanner: View {
    @Environment(AppSessionStore.self) private var session
    @Environment(ToastCenter.self) private var toasts
    @State private var opened: OnboardingNudge?

    private var current: OnboardingNudge? {
        guard !session.nudgeDismissed, let profile = session.profile else { return nil }
        return nudge(for: profile, connectX: AppFeatures.current.connectX)
    }

    private var isOpen: Binding<Bool> {
        Binding(get: { opened != nil }, set: { if !$0 { opened = nil } })
    }

    private var sheetTitle: String {
        if case .linkX = opened { LinkCopy.xTitle } else { LinkCopy.phoneTitle }
    }

    var body: some View {
        content
            .sheet(isPresented: isOpen) {
                Group {
                    switch opened {
                    case .addPhone: NavigationStack { PhoneStepView(mode: .sheet) }
                    case .linkX: NavigationStack { SocialsStepView(mode: .sheet) }
                    case nil: EmptyView()
                    }
                }
                .monacoSheet(title: sheetTitle)
                .monacoToastCenter(toasts)
            }
    }

    @ViewBuilder
    private var content: some View {
        if let current {
            MonacoGroupedList {
                HStack(spacing: 0) {
                    Button {
                        opened = current
                    } label: {
                        HStack(spacing: MonacoTheme.Space.sm) {
                            Text(current.message)
                                .font(MonacoTheme.Typo.rowTitle)
                                .foregroundStyle(MonacoTheme.ink)
                                .multilineTextAlignment(.leading)
                                .frame(maxWidth: .infinity, alignment: .leading)
                            MonacoRowChevron()
                        }
                        .padding(.leading, MonacoTheme.Space.gutter)
                        .padding(.vertical, MonacoTheme.Space.sm)
                        .frame(minHeight: 44)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("onboarding-nudge-open")

                    Button {
                        session.nudgeDismissed = true
                    } label: {
                        Image(systemName: "xmark")
                            .font(MonacoTheme.Typo.captionStrong)
                            .foregroundStyle(MonacoTheme.muted)
                            .frame(width: 44, height: 44)
                            .contentShape(Rectangle())
                    }
                    .accessibilityLabel("Hide")
                    .accessibilityIdentifier("onboarding-nudge-close")
                }
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("onboarding-nudge")
        }
    }
}
