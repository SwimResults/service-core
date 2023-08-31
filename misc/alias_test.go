package misc

import "testing"
import "github.com/stretchr/testify/assert"

func TestAliasifySimple(t *testing.T) {
	s1 := "Konrad Weiß"
	s2 := Aliasify(s1)
	assert.Equal(t, "konradweiss", s2)
}

func TestAliasifyMultipleOccurences(t *testing.T) {
	s1 := "Konradß Wèiß"
	s2 := Aliasify(s1)
	assert.Equal(t, "konradssweiss", s2)
}

func TestAliasifyLarge(t *testing.T) {
	s1 := "fhweuifèèèèèéééééêêêê..gheg.gegelrgergßßßèèßßß-"
	s2 := Aliasify(s1)
	assert.Equal(t, "fhweuifeeeeeeeeeeeeeegheggegelrgergsssssseessssss", s2)
}

func TestAliasifyRegexReplacement(t *testing.T) {
	s1 := "Konradß @%Wèi$$ß"
	s2 := Aliasify(s1)
	assert.Equal(t, "konradssweiss", s2)
}
