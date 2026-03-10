package prompt

func equalStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok {
			return false
		}
		if leftValue != rightValue {
			return false
		}
	}
	return true
}

func stableStaticBuild(build func() string, snapshot func() map[string]string) (string, map[string]string, bool) {
	for attempt := 0; attempt < 3; attempt++ {
		before := snapshot()
		content := build()
		after := snapshot()
		if equalStringMap(before, after) {
			return content, after, true
		}
	}
	return build(), snapshot(), false
}
