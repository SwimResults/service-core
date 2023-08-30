package misc

import "strings"

func Aliasify(text string) string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, " ", "")

	replacements := map[string]string{
		"ß": "ss",
		"-": "",
		".": "",

		"ä": "ae",
		"ö": "oe",
		"ü": "ue",

		"é": "e",
		"è": "e",
		"ê": "e",

		"à": "a",
		"á": "a",
		"â": "a",

		"ò": "o",
		"ó": "o",
		"ô": "o",
	}

	for a, b := range replacements {
		text = strings.ReplaceAll(text, a, b)
	}

	return text
}

func ExtractNames(name string) (bool, string, string) {
	if strings.Contains(name, ",") {
		names := strings.Split(name, ",")
		return true, strings.Trim(names[1], " "), strings.Trim(names[0], " ")
	}
	return false, "", ""
}
