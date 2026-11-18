package commands

import (
	"fmt"
	"strings"
)

// token is one word of the input, with its byte span.
type token struct {
	text       string
	start, end int
}

// tokenize splits s on whitespace. A word starting with " runs to the next "
// (the quotes are dropped from text); an unclosed quote runs to the end.
func tokenize(s string) []token {
	var out []token
	i := 0
	for i < len(s) {
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		if s[i] == '"' {
			j := strings.IndexByte(s[i+1:], '"')
			if j < 0 {
				out = append(out, token{s[i+1:], start, len(s)})
				break
			}
			end := i + 1 + j + 1
			out = append(out, token{s[i+1 : end-1], start, end})
			i = end
			continue
		}
		for i < len(s) && !isSpace(s[i]) {
			i++
		}
		out = append(out, token{s[start:i], start, i})
	}
	return out
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' }

// match finds the command whose name begins input: the longest name made of
// whole words, case-insensitive. n is how many tokens the name used.
func match(app string, toks []token) (spec *Spec, n int) {
	for _, s := range Catalog(app) {
		words := strings.Fields(s.Name)
		if len(words) > len(toks) || (spec != nil && len(words) <= n) {
			continue
		}
		ok := true
		for i, w := range words {
			if !strings.EqualFold(toks[i].text, w) {
				ok = false
				break
			}
		}
		if ok {
			spec, n = s, len(words)
		}
	}
	return spec, n
}

// Parsed is a command line split into its command and argument values.
type Parsed struct {
	Spec *Spec
	Args []string
}

// Parse reads a command line ("tag add work ideas").
func Parse(app, input string) (*Parsed, error) {
	input = strings.TrimSpace(input)
	toks := tokenize(input)
	if len(toks) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	spec, n := match(app, toks)
	if spec == nil {
		return nil, fmt.Errorf("unknown command: %s", toks[0].text)
	}
	args, err := bindArgs(spec, input, toks[n:])
	if err != nil {
		return nil, err
	}
	return &Parsed{Spec: spec, Args: args}, nil
}

// bindArgs assigns tokens to spec's arguments. A Rest argument takes the raw
// remaining text (one pair of surrounding quotes is dropped); a Many argument
// takes every remaining word.
func bindArgs(spec *Spec, input string, toks []token) ([]string, error) {
	var args []string
	i := 0
	for _, a := range spec.Args {
		switch {
		case i >= len(toks):
			if !a.Optional {
				return nil, fmt.Errorf("%s: missing <%s> (usage: %s)", spec.Name, a.Name, spec.Usage())
			}
		case a.Rest:
			raw := strings.TrimSpace(input[toks[i].start:])
			if len(toks)-i == 1 {
				raw = toks[i].text
			}
			args = append(args, raw)
			i = len(toks)
		case a.Many:
			for ; i < len(toks); i++ {
				args = append(args, toks[i].text)
			}
		default:
			args = append(args, toks[i].text)
			i++
		}
	}
	if i < len(toks) {
		return nil, fmt.Errorf("%s: unexpected %q (usage: %s)", spec.Name, toks[i].text, spec.Usage())
	}
	for _, a := range spec.Args {
		if len(a.Choices) == 0 {
			continue
		}
		for _, v := range args {
			if !contains(a.Choices, strings.ToLower(v)) {
				return nil, fmt.Errorf("%s: %q isn't one of %s", spec.Name, v, strings.Join(a.Choices, ", "))
			}
		}
	}
	return args, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
