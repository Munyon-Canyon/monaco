package errs

const CodeDashboardTimeout Code = "dashboard_timeout"

func (codeFiles) AnalyticsDashboards() map[Code]Row {
	return map[Code]Row{
		CodeDashboardTimeout: {
			Name: "DashboardTimeout", Kind: KindUnavailable, Retryable: true,
			Message: "That dashboard took too long to read. Try a shorter range.",
		},
	}
}
