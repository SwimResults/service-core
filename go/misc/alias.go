package misc

import "strings"

func Aliasify(text string) string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, " ", "")

	// TODO: handle special characters

	return text
}
