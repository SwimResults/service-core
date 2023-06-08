package misc

func AppendWithoutDuplicates(a []string, e string) []string {
	found := false
	for _, b := range a {
		if b == e {
			found = true
		}
	}
	if !found {
		return append(a, e)
	}
	return a
}
