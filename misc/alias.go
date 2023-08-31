package misc

import (
	"regexp"
	"strings"
)

func Aliasify(text string) string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, " ", "")

	replacements := map[string]string{
		"ß": "ss",

		"ä": "ae",
		"æ": "ae",
		"ö": "oe",
		"œ": "oe",
		"ü": "ue",

		"é": "e",
		"è": "e",
		"ê": "e",

		"à": "a",
		"á": "a",
		"â": "a",
		"ã": "a",
		"å": "a",
		"ā": "a",

		"ò": "o",
		"ó": "o",
		"ô": "o",
		"õ": "o",
		"ō": "o",
		"ø": "o",

		"û": "u",
		"ù": "u",
		"ú": "u",
		"ū": "u",

		"î": "i",
		"ï": "i",
		"í": "i",
		"ī": "i",
		"ì": "i",

		"ç": "c",
		"ć": "c",
		"č": "c",

		"ś": "s",
		"š": "s",

		"ÿ": "y",

		"ñ": "n",
		"ń": "n",
	}

	for a, b := range replacements {
		text = strings.ReplaceAll(text, a, b)
	}

	text = regexp.MustCompile(`[^a-zA-Z0-9 ]+`).ReplaceAllString(text, "")

	return text
}

func ExtractNames(name string) (bool, string, string) {
	if strings.Contains(name, ",") {
		names := strings.Split(name, ",")
		return true, strings.Trim(names[1], " "), strings.Trim(names[0], " ")
	}
	return false, "", ""
}
