import Testing

struct TaskOnEmptyViewRuleTests {
    @Test
    func flagsTheFeedBeforeItsFix() {
        let source = """
            struct FeedView: View {
                var body: some View {
                    Group {
                        if let model {
                            FeedScreen(model: model)
                        }
                    }
                    .navigationTitle(FeedTab.title)
                    .accessibilityIdentifier("feed-root")
                    .task { await start() }
                }
            }
            """
        #expect(TaskOnEmptyViewRule.violations(in: source) == [3])
    }

    @Test
    func flagsAComputedViewWhoseSwitchCanBeEmpty() {
        let source = """
            struct LeaveSection: View {
                var body: some View {
                    content
                        .padding(.horizontal, 16)
                        .onAppear { load() }
                }

                @ViewBuilder
                private var content: some View {
                    switch model?.standing {
                    case .loaded(let standing):
                        Text(standing.name)
                    case .idle, nil:
                        EmptyView()
                    }
                }
            }
            """
        #expect(TaskOnEmptyViewRule.violations(in: source) == [3])
    }

    @Test
    func passesTheFeedAfterItsFix() {
        let source = """
            struct FeedView: View {
                var body: some View {
                    ZStack {
                        Color.clear
                        if let model {
                            FeedScreen(model: model)
                        }
                    }
                    .task { await start() }
                }
            }
            """
        #expect(TaskOnEmptyViewRule.violations(in: source).isEmpty)
    }

    @Test
    func passesAGroupWithAnElse() {
        let source = """
            struct FeedView: View {
                var body: some View {
                    Group {
                        if let model {
                            FeedScreen(model: model)
                        } else {
                            ProgressView()
                        }
                    }
                    .task { await start() }
                }
            }
            """
        #expect(TaskOnEmptyViewRule.violations(in: source).isEmpty)
    }
}
