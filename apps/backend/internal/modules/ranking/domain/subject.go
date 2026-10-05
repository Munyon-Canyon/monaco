package domain

func SubjectName(displayName, handle string) string {
	if displayName != "" {
		return displayName
	}
	return handle
}
