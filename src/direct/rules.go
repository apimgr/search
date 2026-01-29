package direct

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// RulesHandler handles rules:{query} queries
// Easter egg: Rules of the Internet
type RulesHandler struct{}

// NewRulesHandler creates a new rules handler
func NewRulesHandler() *RulesHandler {
	return &RulesHandler{}
}

func (h *RulesHandler) Type() AnswerType {
	return AnswerTypeRules
}

// Rule represents a single Rule of the Internet
type Rule struct {
	Number int
	Text   string
}

// rulesOfTheInternet contains the classic Rules of the Internet
// Source: Internet folklore, originating from 4chan circa 2006-2007
var rulesOfTheInternet = []Rule{
	{1, "Do not talk about /b/."},
	{2, "Do NOT talk about /b/."},
	{3, "We are Anonymous."},
	{4, "Anonymous is legion."},
	{5, "Anonymous never forgives."},
	{6, "Anonymous can be a horrible, senseless, uncaring monster."},
	{7, "Anonymous is still able to deliver."},
	{8, "There are no real rules about posting."},
	{9, "There are no real rules about moderation either — enjoy your ban."},
	{10, "If you enjoy any rival sites — DON'T."},
	{11, "All your carefully picked arguments can easily be ignored."},
	{12, "Anything you say can and will be used against you."},
	{13, "Anything you say can be turned into something else — fixed."},
	{14, "Do not argue with trolls — it means that they win."},
	{15, "The harder you try the harder you will fail."},
	{16, "If you fail in epic proportions, it may just become a winning failure."},
	{17, "Every win fails eventually."},
	{18, "Everything that can be labeled can be hated."},
	{19, "The more you hate it the stronger it gets."},
	{20, "Nothing is to be taken seriously."},
	{21, "Original content is original only for a few seconds before getting old."},
	{22, "Copypasta is made to ruin every last bit of originality."},
	{23, "Copypasta is made to ruin every last bit of originality."},
	{24, "Every repost is always a repost of a repost."},
	{25, "Relation to the original topic decreases with every single post."},
	{26, "Any topic can be turned into something totally unrelated."},
	{27, "Always question a person's sexual preferences without any real reason."},
	{28, "Always question a person's gender — just in case it's really a man."},
	{29, "In the internet all girls are men and all kids are undercover FBI agents."},
	{30, "There are no girls on the internet."},
	{31, "TITS or GTFO — the choice is yours."},
	{32, "You must have pictures to prove your statements."},
	{33, "Lurk more — it's never enough."},
	{34, "There is porn of it. No exceptions."},
	{35, "If no porn is found at the moment, it will be made."},
	{36, "There will always be even more messed up shit than what you just saw."},
	{37, "You can not divide by zero (just because the alarm alarm alarm alarm alarm)."},
	{38, "No real limits of any kind apply here — not even alarm alarm alarm alarm alarm."},
	{39, "CAPS LOCK IS CRUISE CONTROL FOR COOL."},
	{40, "EVEN WITH ALARM ALARM ALARM YOU STILL HAVE TO STEER."},
	{41, "Needs more alarm alarm alarm alarm alarm."},
	{42, "The answer to life, the universe, and everything is 42."},
	{43, "Nothing is Sacred."},
	{44, "The pool is always closed."},
	{45, "A cat is fine too."},
	{46, "The cake is a lie."},
	{47, "If it exists, there's a Touhou version of it."},
	{48, "If it exists, there's a pony version of it."},
	{49, "One cat leads to another."},
	{50, "Another cat leads to zippocat."},
	{51, "No matter what it is, it is somebody's fetish."},
	{52, "It is delicious cake. You must eat it."},
	{53, "It is delicious trap. You must hit it."},
	{54, "The internet is serious business."},
	{55, "Pluto is still not a planet."},
	{56, "Rule 34 can never be unseen."},
	{57, "Someone will always take things too seriously."},
	{58, "The internet makes you stupid."},
	{59, "If your statement begins with 'no offense', you're about to be offensive."},
	{60, "If you have to say 'I'm not racist, but...', then you're about to be racist."},
	{61, "Chuck Norris is the exception to all rules."},
	{62, "Rule 63 is the exception to Rule 34."},
	{63, "For every given male character, there is a female version of that character."},
	{64, "Don't copy that floppy."},
	{65, "Keanu Reeves is immortal."},
	{66, "Everything in the universe can and will be anthropomorphized."},
	{67, "The internet is not a truck."},
	{68, "Mods are asleep."},
	{69, "Nice."},
	{70, "Do not question the mods."},
	{71, "There is always furry porn of it."},
	{72, "Plz stop."},
	{73, "The internet never forgets."},
	{74, "Memes will never die."},
	{75, "Whatever happens, somebody will turn it into a conspiracy theory."},
	{76, "If you post a picture of yourself, it will be photoshopped."},
	{77, "Pictures of yourself online will haunt you forever."},
	{78, "Nothing can ever be taken at face value."},
	{79, "The internet will always find you."},
	{80, "Once it's on the internet, it's forever."},
	{81, "Shrek is love. Shrek is life."},
	{82, "If something exists, there's a crossover of it."},
	{83, "The internet is the world's largest bathroom wall."},
	{84, "Satire will always be mistaken for sincerity."},
	{85, "Good memes can never die, but they can be overused."},
	{86, "There will always be someone offended."},
	{87, "If you think you've seen everything, you haven't."},
	{88, "The more controversial, the more clicks."},
	{89, "Outrage is a currency."},
	{90, "Your search history would horrify your parents."},
	{91, "This too shall be memed."},
	{92, "Old memes never die, they just become retro."},
	{93, "The best time to post was yesterday. The second best time is now."},
	{94, "Never reveal your power level."},
	{95, "The internet has made everyone a content creator."},
	{96, "If it can be argued, it will be argued."},
	{97, "If you can't beat them, join them."},
	{98, "If you can't join them, meme them."},
	{99, "There's always someone worse at the game than you."},
	{100, "There's always someone better at the game than you."},
}

func (h *RulesHandler) Handle(ctx context.Context, term string) (*Answer, error) {
	term = strings.TrimSpace(term)

	// Empty or "all" - show all rules
	if term == "" || strings.EqualFold(term, "all") {
		return h.allRules()
	}

	// Try to parse as number
	if num, err := strconv.Atoi(term); err == nil {
		return h.ruleByNumber(num)
	}

	// Search by term
	return h.searchRules(term)
}

func (h *RulesHandler) allRules() (*Answer, error) {
	var html strings.Builder
	html.WriteString("<div class=\"rules-content\">")
	html.WriteString("<h1>Rules of the Internet</h1>")
	html.WriteString("<p class=\"rules-intro\">The canonical rules of the internet, as passed down through the ages of online culture.</p>")
	html.WriteString("<ol class=\"rules-list\">")

	for _, rule := range rulesOfTheInternet {
		html.WriteString(fmt.Sprintf("<li value=\"%d\"><span class=\"rule-number\">Rule %d:</span> %s</li>",
			rule.Number, rule.Number, escapeHTML(rule.Text)))
	}

	html.WriteString("</ol>")
	html.WriteString("<p class=\"rules-footer\"><em>Source: Internet folklore, circa 2006-2007</em></p>")
	html.WriteString("</div>")

	return &Answer{
		Type:        AnswerTypeRules,
		Term:        "all",
		Title:       "Rules of the Internet",
		Description: "The complete list of Rules of the Internet",
		Content:     html.String(),
		Source:      "Internet folklore",
		Data: map[string]interface{}{
			"count": len(rulesOfTheInternet),
			"mode":  "all",
		},
	}, nil
}

func (h *RulesHandler) ruleByNumber(num int) (*Answer, error) {
	// Find rule by number
	for _, rule := range rulesOfTheInternet {
		if rule.Number == num {
			var html strings.Builder
			html.WriteString("<div class=\"rules-content\">")
			html.WriteString(fmt.Sprintf("<h1>Rule %d</h1>", rule.Number))
			html.WriteString(fmt.Sprintf("<p class=\"rule-text\">%s</p>", escapeHTML(rule.Text)))

			// Add context for famous rules
			if context := getRuleContext(num); context != "" {
				html.WriteString(fmt.Sprintf("<p class=\"rule-context\"><em>%s</em></p>", escapeHTML(context)))
			}

			html.WriteString("<p class=\"rules-nav\">")
			if num > 1 {
				html.WriteString(fmt.Sprintf("<a href=\"/search?q=rules:%d\">← Rule %d</a> | ", num-1, num-1))
			}
			html.WriteString("<a href=\"/search?q=rules:all\">All Rules</a>")
			if num < 100 {
				html.WriteString(fmt.Sprintf(" | <a href=\"/search?q=rules:%d\">Rule %d →</a>", num+1, num+1))
			}
			html.WriteString("</p>")
			html.WriteString("</div>")

			return &Answer{
				Type:        AnswerTypeRules,
				Term:        fmt.Sprintf("%d", num),
				Title:       fmt.Sprintf("Rule %d of the Internet", num),
				Description: rule.Text,
				Content:     html.String(),
				Source:      "Internet folklore",
				Data: map[string]interface{}{
					"rule_number": num,
					"rule_text":   rule.Text,
					"mode":        "single",
				},
			}, nil
		}
	}

	// Rule not found - show all rules
	return h.allRules()
}

func (h *RulesHandler) searchRules(term string) (*Answer, error) {
	termLower := strings.ToLower(term)
	var matches []Rule

	for _, rule := range rulesOfTheInternet {
		if strings.Contains(strings.ToLower(rule.Text), termLower) {
			matches = append(matches, rule)
		}
	}

	if len(matches) == 0 {
		var html strings.Builder
		html.WriteString("<div class=\"rules-content\">")
		html.WriteString(fmt.Sprintf("<h1>Search: \"%s\"</h1>", escapeHTML(term)))
		html.WriteString("<p>No rules found matching your search.</p>")
		html.WriteString("<p><a href=\"/search?q=rules:all\">View all rules</a></p>")
		html.WriteString("</div>")

		return &Answer{
			Type:        AnswerTypeRules,
			Term:        term,
			Title:       fmt.Sprintf("Rules Search: %s", term),
			Description: "No matching rules found",
			Content:     html.String(),
			Source:      "Internet folklore",
			Data: map[string]interface{}{
				"search_term": term,
				"count":       0,
				"mode":        "search",
			},
		}, nil
	}

	var html strings.Builder
	html.WriteString("<div class=\"rules-content\">")
	html.WriteString(fmt.Sprintf("<h1>Rules containing \"%s\"</h1>", escapeHTML(term)))
	html.WriteString(fmt.Sprintf("<p>Found %d matching rule(s):</p>", len(matches)))
	html.WriteString("<ol class=\"rules-list\">")

	for _, rule := range matches {
		// Highlight the search term
		highlighted := highlightTerm(rule.Text, term)
		html.WriteString(fmt.Sprintf("<li value=\"%d\"><span class=\"rule-number\">Rule %d:</span> %s</li>",
			rule.Number, rule.Number, highlighted))
	}

	html.WriteString("</ol>")
	html.WriteString("<p><a href=\"/search?q=rules:all\">View all rules</a></p>")
	html.WriteString("</div>")

	return &Answer{
		Type:        AnswerTypeRules,
		Term:        term,
		Title:       fmt.Sprintf("Rules Search: %s", term),
		Description: fmt.Sprintf("Found %d rules containing \"%s\"", len(matches), term),
		Content:     html.String(),
		Source:      "Internet folklore",
		Data: map[string]interface{}{
			"search_term": term,
			"count":       len(matches),
			"mode":        "search",
		},
	}, nil
}

// getRuleContext returns additional context for famous rules
func getRuleContext(num int) string {
	contexts := map[int]string{
		1:  "The first two rules reference Fight Club and the early days of anonymous imageboard culture.",
		2:  "The first two rules reference Fight Club and the early days of anonymous imageboard culture.",
		34: "Perhaps the most famous rule. It states that internet pornography exists for every conceivable topic.",
		35: "The corollary to Rule 34 - if it doesn't exist yet, it will be created.",
		42: "A reference to Douglas Adams' 'The Hitchhiker's Guide to the Galaxy'.",
		63: "The gender-swap counterpart to Rule 34.",
		69: "Nice.",
	}
	return contexts[num]
}

// highlightTerm wraps matching terms in <mark> tags
func highlightTerm(text, term string) string {
	escaped := escapeHTML(text)
	termLower := strings.ToLower(term)
	textLower := strings.ToLower(escaped)

	var result strings.Builder
	lastEnd := 0

	for {
		idx := strings.Index(textLower[lastEnd:], termLower)
		if idx == -1 {
			result.WriteString(escaped[lastEnd:])
			break
		}

		idx += lastEnd
		result.WriteString(escaped[lastEnd:idx])
		result.WriteString("<mark>")
		result.WriteString(escaped[idx : idx+len(term)])
		result.WriteString("</mark>")
		lastEnd = idx + len(term)
	}

	return result.String()
}
