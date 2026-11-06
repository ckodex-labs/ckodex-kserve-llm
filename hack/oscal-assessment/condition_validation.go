package main

// unambiguousCondition requires exactly one condition of the requested type.
func unambiguousCondition(service object, kind, reason string) bool {
	found := false
	passed := false
	for _, c := range service.Status.Conditions {
		if c.Type != kind {
			continue
		}
		if found {
			return false
		}
		found = true
		passed = c.Status == "True" && c.Reason == reason
	}
	return found && passed
}
