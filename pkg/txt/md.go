package txt

import (
	"fmt"
	"strings"
	"unicode"
)

type token struct {
	consumed string
	left     string
}

type Parser func(input string) []token

var openTags = map[string]string{
	"*":  "<i>",
	"**": "<b>",
	"_":  "<i>",
	"__": "<b>",
	"`":  "<code>",
}

var closeTags = map[string]string{
	"*":  "</i>",
	"**": "</b>",
	"_":  "</i>",
	"__": "</b>",
	"`":  "</code>",
}

func term(t string) Parser {
	return func(input string) []token {
		if strings.HasPrefix(input, t) {
			return []token{{"[" + t + "]", input[len(t):]}}
		}
		return nil
	}
}

func openTerm(t string) Parser {
	return func(input string) []token {
		if strings.HasPrefix(input, t) {
			return []token{{openTags[t], input[len(t):]}}
		}
		return nil
	}
}

func closeTerm(t string) Parser {
	return func(input string) []token {
		if strings.HasPrefix(input, t) {
			return []token{{closeTags[t], input[len(t):]}}
		}
		return nil
	}
}

func digit() Parser {
	return func(input string) []token {
		if len(input) > 0 && unicode.IsDigit(rune(input[0])) {
			return []token{{string(input[0]), input[1:]}}
		}
		return nil
	}
}

func alphaNumeric() Parser {
	return func(input string) []token {
		if len(input) > 0 && (unicode.IsLetter(rune(input[0])) || unicode.IsDigit(rune(input[0]))) {
			return []token{{string(input[0]), input[1:]}}
		}
		return nil
	}
}

func emptyString() Parser {
	return func(input string) []token {
		if input == "" {
			return []token{{"", input}}
		}
		return nil
	}
}

func or(lhs, rhs Parser) Parser {
	return func(input string) []token {
		return append(lhs(input), rhs(input)...)
	}
}

func and(lhs, rhs Parser) Parser {
	return func(input string) []token {
		var results []token
		for _, litem := range lhs(input) {
			for _, ritem := range rhs(litem.left) {
				if litem.consumed != "" && ritem.consumed != "" {
					results = append(results, token{litem.consumed + ritem.consumed, ritem.left})
				}
			}
		}
		return results
	}
}

func recursive(input string, parser Parser, depth int) []token {
	var results []token
	empty := true
	for _, item := range parser(input) {
		if item.consumed == "" {
			continue
		}
		empty = false
		for _, child := range recursive(item.left, parser, depth+1) {
			results = append(results, token{item.consumed + child.consumed, child.left})
		}
	}
	if empty && depth != 0 {
		results = append(results, token{"", input})
	}

	return results
}

// some applies the parser for more than one time. Each parse result is combined with the previous result.
// And each parse can generate multiple results.
func some(parser Parser) Parser {
	return func(input string) []token {
		return recursive(input, parser, 0)
	}
}

func zeroOrMore(parser Parser) Parser {
	return func(input string) []token {
		return recursive(input, parser, 1)
	}
}

// markdown incrementally yields when it encounters a *, **, _, __ or `
func markdown() Parser {
	return func(input string) []token {
		for i := 0; i < len(input); i++ {
			if input[i] == '*' || input[i] == '_' || input[i] == '`' {
				return []token{{input[:i], input[i:]}}
			}
		}
		if len(input) > 0 && (input[len(input)-1] == '*' || input[len(input)-1] != '_' || input[len(input)-1] != '`') {
			return []token{{input, ""}}
		}
		return nil
	}
}

// Html converts user's markdown to Telegram-supported subset of HTML
// Telegram supported tags:
// <b>bold</b>, <strong>bold</strong>
// <i>italic</i>, <em>italic</em>
// <u>underline</u>, <ins>underline</ins>
// <s>strikethrough</s>, <strike>strikethrough</strike>, <del>strikethrough</del>
// <span class="tg-spoiler">spoiler</span>, <tg-spoiler>spoiler</tg-spoiler>
// <b>bold <i>italic bold <s>italic bold strikethrough <span class="tg-spoiler">italic bold strikethrough spoiler</span></s> <u>underline italic bold</u></i> bold</b>
// <a href="http://www.example.com/">inline URL</a>
// <a href="tg://user?id=123456789">inline mention of a user</a>
// <tg-emoji emoji-id="5368324170671202286">👍</tg-emoji>
// <code>inline fixed-width code</code>
// <pre>pre-formatted fixed-width code block</pre>
// <pre><code class="language-python">pre-formatted fixed-width code block written in the Python programming language</code></pre>
// <blockquote>Block quotation started\nBlock quotation continued\nThe last line of the block quotation</blockquote>
// <blockquote expandable>Expandable block quotation started\nExpandable block quotation continued\nExpandable block quotation continued\nHidden by default part of the block quotation started\nExpandable block quotation continued\nThe last line of the block quotation</blockquote>
func Html(md string) string {
	var htmlEscaper = strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
	)
	md = htmlEscaper.Replace(md)
	// By this point our markdown is safe to send as HTML via Telegram.
	// There won't be any issues like "missing closing HTML tag",
	// for the cases when our markdown has some html tags.
	// We try to convert as much markdown as possible to Telegram HTML.

	text := markdown()
	code := and(openTerm("`"), and(text, closeTerm("`")))
	onlyBold := or(
		and(openTerm("**"), and(or(code, text), closeTerm("**"))),
		and(openTerm("__"), and(or(code, text), closeTerm("__"))),
	)
	italicNoCyclic := or(
		and(openTerm("*"), and(some(or(
			onlyBold,
			or(code, text))),
			closeTerm("*"))),
		and(openTerm("_"), and(some(or(
			onlyBold,
			or(code, text))),
			closeTerm("_"))),
	)
	onlyItalic := or(
		and(openTerm("*"), and(or(code, text), closeTerm("*"))),
		and(openTerm("_"), and(or(code, text), closeTerm("_"))),
	)
	boldNoCyclic := or(
		and(openTerm("**"), and(some(or(
			onlyItalic,
			or(code, text))),
			closeTerm("**"))),
		and(openTerm("__"), and(some(or(
			onlyItalic,
			or(code, text))),
			closeTerm("__"))),
	)
	italic := or(
		and(openTerm("*"), and(some(or(boldNoCyclic, or(code, text))), closeTerm("*"))),
		and(openTerm("_"), and(some(or(boldNoCyclic, or(code, text))), closeTerm("_"))),
	)
	bold := or(
		and(openTerm("**"), and(some(or(italicNoCyclic, or(text, code))), closeTerm("**"))),
		and(openTerm("__"), and(some(or(italicNoCyclic, or(text, code))), closeTerm("__"))),
	)

	span := or(bold, or(italic, or(code, text)))
	doc := some(span)

	for _, tok := range doc(md) {
		fmt.Printf("%v\n", tok.consumed)
	}

	// TODO only return if remained is empty?
	for _, tok := range doc(md) {
		return tok.consumed
	}

	// If we can't consume md, return unchanged
	return md
}
