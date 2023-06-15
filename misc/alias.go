package misc

import "strings"

func Aliasify(text string) string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, " ", "")

	// TODO: handle special characters

	return text
}

func ExtractNames(name string) (bool, string, string) {
	if strings.Contains(name, ",") {
		names := strings.Split(name, ",")
		return true, strings.Trim(names[1], " "), strings.Trim(names[0], " ")
	}
	return false, "", ""
}
