package instant

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// EmojiHandler handles emoji lookups by name/keyword
type EmojiHandler struct {
	patterns []*regexp.Regexp
	emojis   map[string]EmojiInfo
}

// EmojiInfo contains information about an emoji
type EmojiInfo struct {
	Emoji    string
	Name     string
	Keywords []string
	Category string
}

// NewEmojiHandler creates a new emoji handler
func NewEmojiHandler() *EmojiHandler {
	return &EmojiHandler{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^emoji[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^emojis?[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^find\s+emoji[:\s]+(.+)$`),
			regexp.MustCompile(`(?i)^search\s+emoji[:\s]+(.+)$`),
		},
		emojis: getCommonEmojis(),
	}
}

func (h *EmojiHandler) Name() string {
	return "emoji"
}

func (h *EmojiHandler) Patterns() []*regexp.Regexp {
	return h.patterns
}

func (h *EmojiHandler) CanHandle(query string) bool {
	for _, p := range h.patterns {
		if p.MatchString(query) {
			return true
		}
	}
	return false
}

func (h *EmojiHandler) Handle(ctx context.Context, query string) (*Answer, error) {
	var searchTerm string
	for _, p := range h.patterns {
		if matches := p.FindStringSubmatch(query); len(matches) > 1 {
			searchTerm = strings.TrimSpace(strings.ToLower(matches[1]))
			break
		}
	}

	if searchTerm == "" {
		return nil, nil
	}

	// Search for matching emojis
	matches := h.searchEmojis(searchTerm)

	if len(matches) == 0 {
		return &Answer{
			Type:    AnswerTypeEmoji,
			Query:   query,
			Title:   fmt.Sprintf("Emoji Search: %s", searchTerm),
			Content: fmt.Sprintf("No emojis found for '%s'", searchTerm),
		}, nil
	}

	// Build content
	var content strings.Builder
	content.WriteString(fmt.Sprintf("<div class=\"emoji-result\">\n"))
	content.WriteString(fmt.Sprintf("<p>Found <strong>%d</strong> emojis matching \"%s\":</p>\n", len(matches), searchTerm))
	content.WriteString("<div class=\"emoji-grid\" style=\"display: flex; flex-wrap: wrap; gap: 10px;\">\n")

	for _, emoji := range matches {
		content.WriteString(fmt.Sprintf(
			"<div class=\"emoji-item\" style=\"text-align: center; padding: 10px; border: 1px solid #ddd; border-radius: 8px; min-width: 80px;\">\n"+
				"<div style=\"font-size: 32px;\">%s</div>\n"+
				"<div style=\"font-size: 12px;\">%s</div>\n"+
				"</div>\n",
			emoji.Emoji, emoji.Name))
	}

	content.WriteString("</div>\n")
	content.WriteString("</div>")

	// Build data for API response
	emojiData := make([]map[string]interface{}, len(matches))
	for i, emoji := range matches {
		emojiData[i] = map[string]interface{}{
			"emoji":    emoji.Emoji,
			"name":     emoji.Name,
			"keywords": emoji.Keywords,
			"category": emoji.Category,
		}
	}

	return &Answer{
		Type:    AnswerTypeEmoji,
		Query:   query,
		Title:   fmt.Sprintf("Emoji Search: %s", searchTerm),
		Content: content.String(),
		Data: map[string]interface{}{
			"searchTerm": searchTerm,
			"count":      len(matches),
			"emojis":     emojiData,
		},
	}, nil
}

// searchEmojis searches for emojis by name or keyword
func (h *EmojiHandler) searchEmojis(searchTerm string) []EmojiInfo {
	searchTerm = strings.ToLower(searchTerm)
	var results []EmojiInfo
	scores := make(map[string]int)

	for key, emoji := range h.emojis {
		score := 0

		// Exact name match
		if strings.ToLower(emoji.Name) == searchTerm {
			score = 100
		} else if strings.Contains(strings.ToLower(emoji.Name), searchTerm) {
			score = 50
		}

		// Keyword match
		for _, kw := range emoji.Keywords {
			if kw == searchTerm {
				score += 80
				break
			} else if strings.Contains(kw, searchTerm) {
				score += 30
				break
			}
		}

		// Category match
		if strings.Contains(strings.ToLower(emoji.Category), searchTerm) {
			score += 10
		}

		if score > 0 {
			results = append(results, emoji)
			scores[key] = score
		}
	}

	// Sort by score
	sort.Slice(results, func(i, j int) bool {
		return scores[results[i].Emoji] > scores[results[j].Emoji]
	})

	// Limit results
	if len(results) > 20 {
		results = results[:20]
	}

	return results
}

// getCommonEmojis returns a map of common emojis
func getCommonEmojis() map[string]EmojiInfo {
	return map[string]EmojiInfo{
		// Smileys & Emotion
		"grinning":          {Emoji: "\U0001F600", Name: "grinning", Keywords: []string{"smile", "happy", "face"}, Category: "Smileys"},
		"smile":             {Emoji: "\U0001F604", Name: "smile", Keywords: []string{"happy", "joy", "grin"}, Category: "Smileys"},
		"joy":               {Emoji: "\U0001F602", Name: "joy", Keywords: []string{"laugh", "crying", "happy", "lol"}, Category: "Smileys"},
		"rofl":              {Emoji: "\U0001F923", Name: "rofl", Keywords: []string{"laugh", "rolling", "floor"}, Category: "Smileys"},
		"wink":              {Emoji: "\U0001F609", Name: "wink", Keywords: []string{"flirt", "playful"}, Category: "Smileys"},
		"blush":             {Emoji: "\U0001F60A", Name: "blush", Keywords: []string{"shy", "happy", "smile"}, Category: "Smileys"},
		"innocent":          {Emoji: "\U0001F607", Name: "innocent", Keywords: []string{"angel", "halo"}, Category: "Smileys"},
		"heart_eyes":        {Emoji: "\U0001F60D", Name: "heart_eyes", Keywords: []string{"love", "adore", "crush"}, Category: "Smileys"},
		"star_struck":       {Emoji: "\U0001F929", Name: "star_struck", Keywords: []string{"amazing", "wow", "stars"}, Category: "Smileys"},
		"kissing":           {Emoji: "\U0001F617", Name: "kissing", Keywords: []string{"kiss", "love"}, Category: "Smileys"},
		"kissing_heart":     {Emoji: "\U0001F618", Name: "kissing_heart", Keywords: []string{"kiss", "love", "heart"}, Category: "Smileys"},
		"yum":               {Emoji: "\U0001F60B", Name: "yum", Keywords: []string{"delicious", "tasty", "tongue"}, Category: "Smileys"},
		"sunglasses":        {Emoji: "\U0001F60E", Name: "sunglasses", Keywords: []string{"cool", "awesome"}, Category: "Smileys"},
		"thinking":          {Emoji: "\U0001F914", Name: "thinking", Keywords: []string{"hmm", "wonder", "ponder"}, Category: "Smileys"},
		"raised_eyebrow":    {Emoji: "\U0001F928", Name: "raised_eyebrow", Keywords: []string{"skeptical", "doubt"}, Category: "Smileys"},
		"neutral":           {Emoji: "\U0001F610", Name: "neutral", Keywords: []string{"meh", "blank"}, Category: "Smileys"},
		"expressionless":    {Emoji: "\U0001F611", Name: "expressionless", Keywords: []string{"blank", "meh"}, Category: "Smileys"},
		"rolling_eyes":      {Emoji: "\U0001F644", Name: "rolling_eyes", Keywords: []string{"annoyed", "whatever"}, Category: "Smileys"},
		"smirk":             {Emoji: "\U0001F60F", Name: "smirk", Keywords: []string{"sly", "smug"}, Category: "Smileys"},
		"persevere":         {Emoji: "\U0001F623", Name: "persevere", Keywords: []string{"struggle", "frustrated"}, Category: "Smileys"},
		"disappointed":      {Emoji: "\U0001F61E", Name: "disappointed", Keywords: []string{"sad", "upset"}, Category: "Smileys"},
		"worried":           {Emoji: "\U0001F61F", Name: "worried", Keywords: []string{"nervous", "anxious"}, Category: "Smileys"},
		"angry":             {Emoji: "\U0001F620", Name: "angry", Keywords: []string{"mad", "annoyed"}, Category: "Smileys"},
		"rage":              {Emoji: "\U0001F621", Name: "rage", Keywords: []string{"furious", "angry"}, Category: "Smileys"},
		"cry":               {Emoji: "\U0001F622", Name: "cry", Keywords: []string{"sad", "tears", "upset"}, Category: "Smileys"},
		"sob":               {Emoji: "\U0001F62D", Name: "sob", Keywords: []string{"crying", "sad", "tears"}, Category: "Smileys"},
		"scream":            {Emoji: "\U0001F631", Name: "scream", Keywords: []string{"scared", "horror", "shocked"}, Category: "Smileys"},
		"flushed":           {Emoji: "\U0001F633", Name: "flushed", Keywords: []string{"embarrassed", "surprised"}, Category: "Smileys"},
		"dizzy":             {Emoji: "\U0001F635", Name: "dizzy", Keywords: []string{"confused", "spiral"}, Category: "Smileys"},
		"exploding_head":    {Emoji: "\U0001F92F", Name: "exploding_head", Keywords: []string{"mind blown", "shocked"}, Category: "Smileys"},
		"cowboy":            {Emoji: "\U0001F920", Name: "cowboy", Keywords: []string{"western", "hat"}, Category: "Smileys"},
		"party":             {Emoji: "\U0001F973", Name: "party", Keywords: []string{"celebration", "birthday"}, Category: "Smileys"},
		"nerd":              {Emoji: "\U0001F913", Name: "nerd", Keywords: []string{"geek", "glasses"}, Category: "Smileys"},
		"mask":              {Emoji: "\U0001F637", Name: "mask", Keywords: []string{"sick", "covid", "medical"}, Category: "Smileys"},
		"sleeping":          {Emoji: "\U0001F634", Name: "sleeping", Keywords: []string{"tired", "zzz", "sleep"}, Category: "Smileys"},
		"drool":             {Emoji: "\U0001F924", Name: "drool", Keywords: []string{"yummy", "want"}, Category: "Smileys"},
		"clown":             {Emoji: "\U0001F921", Name: "clown", Keywords: []string{"circus", "funny"}, Category: "Smileys"},
		"poop":              {Emoji: "\U0001F4A9", Name: "poop", Keywords: []string{"shit", "crap"}, Category: "Smileys"},
		"ghost":             {Emoji: "\U0001F47B", Name: "ghost", Keywords: []string{"spooky", "halloween"}, Category: "Smileys"},
		"skull":             {Emoji: "\U0001F480", Name: "skull", Keywords: []string{"dead", "death", "skeleton"}, Category: "Smileys"},
		"alien":             {Emoji: "\U0001F47D", Name: "alien", Keywords: []string{"space", "extraterrestrial"}, Category: "Smileys"},
		"robot":             {Emoji: "\U0001F916", Name: "robot", Keywords: []string{"machine", "android"}, Category: "Smileys"},

		// Gestures & Body Parts
		"thumbsup":          {Emoji: "\U0001F44D", Name: "thumbsup", Keywords: []string{"like", "approve", "yes", "ok"}, Category: "Gestures"},
		"thumbsdown":        {Emoji: "\U0001F44E", Name: "thumbsdown", Keywords: []string{"dislike", "no", "disapprove"}, Category: "Gestures"},
		"ok_hand":           {Emoji: "\U0001F44C", Name: "ok_hand", Keywords: []string{"perfect", "okay"}, Category: "Gestures"},
		"pinching":          {Emoji: "\U0001F90F", Name: "pinching", Keywords: []string{"small", "tiny"}, Category: "Gestures"},
		"victory":           {Emoji: "\u270C\uFE0F", Name: "victory", Keywords: []string{"peace", "two", "v"}, Category: "Gestures"},
		"crossed_fingers":   {Emoji: "\U0001F91E", Name: "crossed_fingers", Keywords: []string{"luck", "hope"}, Category: "Gestures"},
		"love_you":          {Emoji: "\U0001F91F", Name: "love_you", Keywords: []string{"ily", "sign language"}, Category: "Gestures"},
		"rock":              {Emoji: "\U0001F918", Name: "rock", Keywords: []string{"metal", "horns"}, Category: "Gestures"},
		"call_me":           {Emoji: "\U0001F919", Name: "call_me", Keywords: []string{"phone", "shaka"}, Category: "Gestures"},
		"point_up":          {Emoji: "\u261D\uFE0F", Name: "point_up", Keywords: []string{"finger", "up"}, Category: "Gestures"},
		"point_down":        {Emoji: "\U0001F447", Name: "point_down", Keywords: []string{"finger", "down"}, Category: "Gestures"},
		"point_left":        {Emoji: "\U0001F448", Name: "point_left", Keywords: []string{"finger", "left"}, Category: "Gestures"},
		"point_right":       {Emoji: "\U0001F449", Name: "point_right", Keywords: []string{"finger", "right"}, Category: "Gestures"},
		"middle_finger":     {Emoji: "\U0001F595", Name: "middle_finger", Keywords: []string{"flip", "rude"}, Category: "Gestures"},
		"raised_hand":       {Emoji: "\u270B", Name: "raised_hand", Keywords: []string{"stop", "high five"}, Category: "Gestures"},
		"wave":              {Emoji: "\U0001F44B", Name: "wave", Keywords: []string{"hello", "bye", "hi"}, Category: "Gestures"},
		"clap":              {Emoji: "\U0001F44F", Name: "clap", Keywords: []string{"applause", "bravo"}, Category: "Gestures"},
		"handshake":         {Emoji: "\U0001F91D", Name: "handshake", Keywords: []string{"deal", "agree"}, Category: "Gestures"},
		"pray":              {Emoji: "\U0001F64F", Name: "pray", Keywords: []string{"thanks", "please", "hope", "namaste"}, Category: "Gestures"},
		"writing":           {Emoji: "\u270D\uFE0F", Name: "writing", Keywords: []string{"write", "pen"}, Category: "Gestures"},
		"muscle":            {Emoji: "\U0001F4AA", Name: "muscle", Keywords: []string{"strong", "flex", "bicep"}, Category: "Gestures"},
		"brain":             {Emoji: "\U0001F9E0", Name: "brain", Keywords: []string{"smart", "think"}, Category: "Body"},
		"eye":               {Emoji: "\U0001F441\uFE0F", Name: "eye", Keywords: []string{"see", "look"}, Category: "Body"},
		"eyes":              {Emoji: "\U0001F440", Name: "eyes", Keywords: []string{"see", "look", "watching"}, Category: "Body"},
		"ear":               {Emoji: "\U0001F442", Name: "ear", Keywords: []string{"hear", "listen"}, Category: "Body"},
		"nose":              {Emoji: "\U0001F443", Name: "nose", Keywords: []string{"smell"}, Category: "Body"},
		"tongue":            {Emoji: "\U0001F445", Name: "tongue", Keywords: []string{"taste", "lick"}, Category: "Body"},
		"lips":              {Emoji: "\U0001F444", Name: "lips", Keywords: []string{"mouth", "kiss"}, Category: "Body"},

		// Hearts & Love
		"heart":             {Emoji: "\u2764\uFE0F", Name: "heart", Keywords: []string{"love", "red"}, Category: "Hearts"},
		"orange_heart":      {Emoji: "\U0001F9E1", Name: "orange_heart", Keywords: []string{"love", "orange"}, Category: "Hearts"},
		"yellow_heart":      {Emoji: "\U0001F49B", Name: "yellow_heart", Keywords: []string{"love", "yellow"}, Category: "Hearts"},
		"green_heart":       {Emoji: "\U0001F49A", Name: "green_heart", Keywords: []string{"love", "green"}, Category: "Hearts"},
		"blue_heart":        {Emoji: "\U0001F499", Name: "blue_heart", Keywords: []string{"love", "blue"}, Category: "Hearts"},
		"purple_heart":      {Emoji: "\U0001F49C", Name: "purple_heart", Keywords: []string{"love", "purple"}, Category: "Hearts"},
		"black_heart":       {Emoji: "\U0001F5A4", Name: "black_heart", Keywords: []string{"love", "dark"}, Category: "Hearts"},
		"white_heart":       {Emoji: "\U0001F90D", Name: "white_heart", Keywords: []string{"love", "pure"}, Category: "Hearts"},
		"broken_heart":      {Emoji: "\U0001F494", Name: "broken_heart", Keywords: []string{"sad", "heartbreak"}, Category: "Hearts"},
		"sparkling_heart":   {Emoji: "\U0001F496", Name: "sparkling_heart", Keywords: []string{"love", "sparkle"}, Category: "Hearts"},
		"two_hearts":        {Emoji: "\U0001F495", Name: "two_hearts", Keywords: []string{"love", "couple"}, Category: "Hearts"},
		"revolving_hearts":  {Emoji: "\U0001F49E", Name: "revolving_hearts", Keywords: []string{"love", "romance"}, Category: "Hearts"},
		"heartbeat":         {Emoji: "\U0001F493", Name: "heartbeat", Keywords: []string{"love", "pulse"}, Category: "Hearts"},
		"kiss_mark":         {Emoji: "\U0001F48B", Name: "kiss_mark", Keywords: []string{"love", "lipstick"}, Category: "Hearts"},
		"cupid":             {Emoji: "\U0001F498", Name: "cupid", Keywords: []string{"love", "arrow"}, Category: "Hearts"},

		// Animals
		"dog":               {Emoji: "\U0001F436", Name: "dog", Keywords: []string{"puppy", "pet"}, Category: "Animals"},
		"cat":               {Emoji: "\U0001F431", Name: "cat", Keywords: []string{"kitty", "pet"}, Category: "Animals"},
		"mouse":             {Emoji: "\U0001F42D", Name: "mouse", Keywords: []string{"rodent"}, Category: "Animals"},
		"hamster":           {Emoji: "\U0001F439", Name: "hamster", Keywords: []string{"pet"}, Category: "Animals"},
		"rabbit":            {Emoji: "\U0001F430", Name: "rabbit", Keywords: []string{"bunny"}, Category: "Animals"},
		"fox":               {Emoji: "\U0001F98A", Name: "fox", Keywords: []string{"wild"}, Category: "Animals"},
		"bear":              {Emoji: "\U0001F43B", Name: "bear", Keywords: []string{"wild"}, Category: "Animals"},
		"panda":             {Emoji: "\U0001F43C", Name: "panda", Keywords: []string{"china"}, Category: "Animals"},
		"koala":             {Emoji: "\U0001F428", Name: "koala", Keywords: []string{"australia"}, Category: "Animals"},
		"tiger":             {Emoji: "\U0001F42F", Name: "tiger", Keywords: []string{"wild"}, Category: "Animals"},
		"lion":              {Emoji: "\U0001F981", Name: "lion", Keywords: []string{"wild", "king"}, Category: "Animals"},
		"cow":               {Emoji: "\U0001F42E", Name: "cow", Keywords: []string{"farm"}, Category: "Animals"},
		"pig":               {Emoji: "\U0001F437", Name: "pig", Keywords: []string{"farm"}, Category: "Animals"},
		"frog":              {Emoji: "\U0001F438", Name: "frog", Keywords: []string{"toad"}, Category: "Animals"},
		"monkey":            {Emoji: "\U0001F435", Name: "monkey", Keywords: []string{"ape"}, Category: "Animals"},
		"chicken":           {Emoji: "\U0001F414", Name: "chicken", Keywords: []string{"bird", "farm"}, Category: "Animals"},
		"penguin":           {Emoji: "\U0001F427", Name: "penguin", Keywords: []string{"bird", "cold"}, Category: "Animals"},
		"bird":              {Emoji: "\U0001F426", Name: "bird", Keywords: []string{"fly"}, Category: "Animals"},
		"eagle":             {Emoji: "\U0001F985", Name: "eagle", Keywords: []string{"bird", "america"}, Category: "Animals"},
		"duck":              {Emoji: "\U0001F986", Name: "duck", Keywords: []string{"bird", "quack"}, Category: "Animals"},
		"owl":               {Emoji: "\U0001F989", Name: "owl", Keywords: []string{"bird", "night"}, Category: "Animals"},
		"bat":               {Emoji: "\U0001F987", Name: "bat", Keywords: []string{"vampire"}, Category: "Animals"},
		"wolf":              {Emoji: "\U0001F43A", Name: "wolf", Keywords: []string{"wild"}, Category: "Animals"},
		"horse":             {Emoji: "\U0001F434", Name: "horse", Keywords: []string{"pony"}, Category: "Animals"},
		"unicorn":           {Emoji: "\U0001F984", Name: "unicorn", Keywords: []string{"magic", "fantasy"}, Category: "Animals"},
		"bee":               {Emoji: "\U0001F41D", Name: "bee", Keywords: []string{"insect", "honey"}, Category: "Animals"},
		"butterfly":         {Emoji: "\U0001F98B", Name: "butterfly", Keywords: []string{"insect", "pretty"}, Category: "Animals"},
		"snail":             {Emoji: "\U0001F40C", Name: "snail", Keywords: []string{"slow"}, Category: "Animals"},
		"bug":               {Emoji: "\U0001F41B", Name: "bug", Keywords: []string{"insect"}, Category: "Animals"},
		"ant":               {Emoji: "\U0001F41C", Name: "ant", Keywords: []string{"insect"}, Category: "Animals"},
		"spider":            {Emoji: "\U0001F577\uFE0F", Name: "spider", Keywords: []string{"insect", "web"}, Category: "Animals"},
		"turtle":            {Emoji: "\U0001F422", Name: "turtle", Keywords: []string{"slow", "shell"}, Category: "Animals"},
		"snake":             {Emoji: "\U0001F40D", Name: "snake", Keywords: []string{"reptile"}, Category: "Animals"},
		"dragon":            {Emoji: "\U0001F409", Name: "dragon", Keywords: []string{"fantasy", "fire"}, Category: "Animals"},
		"dinosaur":          {Emoji: "\U0001F995", Name: "dinosaur", Keywords: []string{"prehistoric", "t-rex"}, Category: "Animals"},
		"whale":             {Emoji: "\U0001F433", Name: "whale", Keywords: []string{"ocean", "sea"}, Category: "Animals"},
		"dolphin":           {Emoji: "\U0001F42C", Name: "dolphin", Keywords: []string{"ocean", "sea"}, Category: "Animals"},
		"fish":              {Emoji: "\U0001F41F", Name: "fish", Keywords: []string{"ocean", "sea"}, Category: "Animals"},
		"shark":             {Emoji: "\U0001F988", Name: "shark", Keywords: []string{"ocean", "scary"}, Category: "Animals"},
		"octopus":           {Emoji: "\U0001F419", Name: "octopus", Keywords: []string{"ocean", "tentacles"}, Category: "Animals"},
		"crab":              {Emoji: "\U0001F980", Name: "crab", Keywords: []string{"ocean", "beach"}, Category: "Animals"},
		"shrimp":            {Emoji: "\U0001F990", Name: "shrimp", Keywords: []string{"ocean", "seafood"}, Category: "Animals"},
		"elephant":          {Emoji: "\U0001F418", Name: "elephant", Keywords: []string{"big", "trunk"}, Category: "Animals"},
		"gorilla":           {Emoji: "\U0001F98D", Name: "gorilla", Keywords: []string{"ape", "strong"}, Category: "Animals"},
		"deer":              {Emoji: "\U0001F98C", Name: "deer", Keywords: []string{"wild", "antlers"}, Category: "Animals"},
		"giraffe":           {Emoji: "\U0001F992", Name: "giraffe", Keywords: []string{"tall", "africa"}, Category: "Animals"},

		// Food & Drink
		"apple":             {Emoji: "\U0001F34E", Name: "apple", Keywords: []string{"fruit", "red"}, Category: "Food"},
		"banana":            {Emoji: "\U0001F34C", Name: "banana", Keywords: []string{"fruit", "yellow"}, Category: "Food"},
		"orange":            {Emoji: "\U0001F34A", Name: "orange", Keywords: []string{"fruit"}, Category: "Food"},
		"lemon":             {Emoji: "\U0001F34B", Name: "lemon", Keywords: []string{"fruit", "sour"}, Category: "Food"},
		"watermelon":        {Emoji: "\U0001F349", Name: "watermelon", Keywords: []string{"fruit", "summer"}, Category: "Food"},
		"grapes":            {Emoji: "\U0001F347", Name: "grapes", Keywords: []string{"fruit", "wine"}, Category: "Food"},
		"strawberry":        {Emoji: "\U0001F353", Name: "strawberry", Keywords: []string{"fruit", "red"}, Category: "Food"},
		"peach":             {Emoji: "\U0001F351", Name: "peach", Keywords: []string{"fruit", "butt"}, Category: "Food"},
		"cherry":            {Emoji: "\U0001F352", Name: "cherry", Keywords: []string{"fruit", "red"}, Category: "Food"},
		"avocado":           {Emoji: "\U0001F951", Name: "avocado", Keywords: []string{"fruit", "green"}, Category: "Food"},
		"tomato":            {Emoji: "\U0001F345", Name: "tomato", Keywords: []string{"vegetable", "red"}, Category: "Food"},
		"eggplant":          {Emoji: "\U0001F346", Name: "eggplant", Keywords: []string{"vegetable", "purple"}, Category: "Food"},
		"carrot":            {Emoji: "\U0001F955", Name: "carrot", Keywords: []string{"vegetable", "orange"}, Category: "Food"},
		"corn":              {Emoji: "\U0001F33D", Name: "corn", Keywords: []string{"vegetable", "yellow"}, Category: "Food"},
		"pepper":            {Emoji: "\U0001F336\uFE0F", Name: "pepper", Keywords: []string{"spicy", "hot"}, Category: "Food"},
		"broccoli":          {Emoji: "\U0001F966", Name: "broccoli", Keywords: []string{"vegetable", "green"}, Category: "Food"},
		"pizza":             {Emoji: "\U0001F355", Name: "pizza", Keywords: []string{"food", "italian"}, Category: "Food"},
		"hamburger":         {Emoji: "\U0001F354", Name: "hamburger", Keywords: []string{"food", "burger"}, Category: "Food"},
		"fries":             {Emoji: "\U0001F35F", Name: "fries", Keywords: []string{"food", "french"}, Category: "Food"},
		"hotdog":            {Emoji: "\U0001F32D", Name: "hotdog", Keywords: []string{"food"}, Category: "Food"},
		"sandwich":          {Emoji: "\U0001F96A", Name: "sandwich", Keywords: []string{"food", "bread"}, Category: "Food"},
		"taco":              {Emoji: "\U0001F32E", Name: "taco", Keywords: []string{"food", "mexican"}, Category: "Food"},
		"burrito":           {Emoji: "\U0001F32F", Name: "burrito", Keywords: []string{"food", "mexican"}, Category: "Food"},
		"sushi":             {Emoji: "\U0001F363", Name: "sushi", Keywords: []string{"food", "japanese"}, Category: "Food"},
		"ramen":             {Emoji: "\U0001F35C", Name: "ramen", Keywords: []string{"food", "noodles", "japanese"}, Category: "Food"},
		"spaghetti":         {Emoji: "\U0001F35D", Name: "spaghetti", Keywords: []string{"food", "pasta", "italian"}, Category: "Food"},
		"bread":             {Emoji: "\U0001F35E", Name: "bread", Keywords: []string{"food", "toast"}, Category: "Food"},
		"cheese":            {Emoji: "\U0001F9C0", Name: "cheese", Keywords: []string{"food"}, Category: "Food"},
		"egg":               {Emoji: "\U0001F95A", Name: "egg", Keywords: []string{"food", "breakfast"}, Category: "Food"},
		"bacon":             {Emoji: "\U0001F953", Name: "bacon", Keywords: []string{"food", "meat"}, Category: "Food"},
		"steak":             {Emoji: "\U0001F969", Name: "steak", Keywords: []string{"food", "meat"}, Category: "Food"},
		"poultry":           {Emoji: "\U0001F357", Name: "poultry", Keywords: []string{"food", "chicken"}, Category: "Food"},
		"cake":              {Emoji: "\U0001F370", Name: "cake", Keywords: []string{"dessert", "birthday"}, Category: "Food"},
		"birthday_cake":     {Emoji: "\U0001F382", Name: "birthday_cake", Keywords: []string{"dessert", "party"}, Category: "Food"},
		"cookie":            {Emoji: "\U0001F36A", Name: "cookie", Keywords: []string{"dessert", "sweet"}, Category: "Food"},
		"chocolate":         {Emoji: "\U0001F36B", Name: "chocolate", Keywords: []string{"dessert", "candy"}, Category: "Food"},
		"candy":             {Emoji: "\U0001F36C", Name: "candy", Keywords: []string{"sweet"}, Category: "Food"},
		"donut":             {Emoji: "\U0001F369", Name: "donut", Keywords: []string{"dessert", "doughnut"}, Category: "Food"},
		"ice_cream":         {Emoji: "\U0001F368", Name: "ice_cream", Keywords: []string{"dessert", "cold"}, Category: "Food"},
		"coffee":            {Emoji: "\u2615", Name: "coffee", Keywords: []string{"drink", "hot", "caffeine"}, Category: "Food"},
		"tea":               {Emoji: "\U0001F375", Name: "tea", Keywords: []string{"drink", "hot"}, Category: "Food"},
		"beer":              {Emoji: "\U0001F37A", Name: "beer", Keywords: []string{"drink", "alcohol"}, Category: "Food"},
		"wine":              {Emoji: "\U0001F377", Name: "wine", Keywords: []string{"drink", "alcohol"}, Category: "Food"},
		"cocktail":          {Emoji: "\U0001F378", Name: "cocktail", Keywords: []string{"drink", "alcohol"}, Category: "Food"},
		"tropical_drink":    {Emoji: "\U0001F379", Name: "tropical_drink", Keywords: []string{"drink", "summer"}, Category: "Food"},
		"champagne":         {Emoji: "\U0001F37E", Name: "champagne", Keywords: []string{"drink", "celebrate"}, Category: "Food"},
		"milk":              {Emoji: "\U0001F95B", Name: "milk", Keywords: []string{"drink", "dairy"}, Category: "Food"},
		"water":             {Emoji: "\U0001F4A7", Name: "water", Keywords: []string{"drink", "drop"}, Category: "Food"},

		// Nature & Weather
		"sun":               {Emoji: "\u2600\uFE0F", Name: "sun", Keywords: []string{"weather", "hot", "sunny"}, Category: "Nature"},
		"moon":              {Emoji: "\U0001F319", Name: "moon", Keywords: []string{"night", "crescent"}, Category: "Nature"},
		"star":              {Emoji: "\u2B50", Name: "star", Keywords: []string{"night", "shine"}, Category: "Nature"},
		"stars":             {Emoji: "\U0001F31F", Name: "stars", Keywords: []string{"night", "glowing"}, Category: "Nature"},
		"cloud":             {Emoji: "\u2601\uFE0F", Name: "cloud", Keywords: []string{"weather", "sky"}, Category: "Nature"},
		"rain":              {Emoji: "\U0001F327\uFE0F", Name: "rain", Keywords: []string{"weather", "wet"}, Category: "Nature"},
		"thunder":           {Emoji: "\u26A1", Name: "thunder", Keywords: []string{"weather", "lightning", "storm"}, Category: "Nature"},
		"snow":              {Emoji: "\u2744\uFE0F", Name: "snow", Keywords: []string{"weather", "cold", "winter"}, Category: "Nature"},
		"snowman":           {Emoji: "\u26C4", Name: "snowman", Keywords: []string{"winter", "cold"}, Category: "Nature"},
		"fire":              {Emoji: "\U0001F525", Name: "fire", Keywords: []string{"hot", "flame", "lit"}, Category: "Nature"},
		"rainbow":           {Emoji: "\U0001F308", Name: "rainbow", Keywords: []string{"weather", "colorful"}, Category: "Nature"},
		"tornado":           {Emoji: "\U0001F32A\uFE0F", Name: "tornado", Keywords: []string{"weather", "storm"}, Category: "Nature"},
		"wind":              {Emoji: "\U0001F32C\uFE0F", Name: "wind", Keywords: []string{"weather", "blow"}, Category: "Nature"},
		"fog":               {Emoji: "\U0001F32B\uFE0F", Name: "fog", Keywords: []string{"weather", "mist"}, Category: "Nature"},
		"ocean":             {Emoji: "\U0001F30A", Name: "ocean", Keywords: []string{"water", "wave", "sea"}, Category: "Nature"},
		"flower":            {Emoji: "\U0001F33C", Name: "flower", Keywords: []string{"plant", "blossom"}, Category: "Nature"},
		"rose":              {Emoji: "\U0001F339", Name: "rose", Keywords: []string{"flower", "love"}, Category: "Nature"},
		"sunflower":         {Emoji: "\U0001F33B", Name: "sunflower", Keywords: []string{"flower", "yellow"}, Category: "Nature"},
		"tree":              {Emoji: "\U0001F333", Name: "tree", Keywords: []string{"plant", "nature"}, Category: "Nature"},
		"palm_tree":         {Emoji: "\U0001F334", Name: "palm_tree", Keywords: []string{"beach", "tropical"}, Category: "Nature"},
		"cactus":            {Emoji: "\U0001F335", Name: "cactus", Keywords: []string{"desert", "plant"}, Category: "Nature"},
		"leaf":              {Emoji: "\U0001F343", Name: "leaf", Keywords: []string{"plant", "wind"}, Category: "Nature"},
		"fallen_leaf":       {Emoji: "\U0001F342", Name: "fallen_leaf", Keywords: []string{"autumn", "fall"}, Category: "Nature"},
		"maple_leaf":        {Emoji: "\U0001F341", Name: "maple_leaf", Keywords: []string{"canada", "autumn"}, Category: "Nature"},
		"mushroom":          {Emoji: "\U0001F344", Name: "mushroom", Keywords: []string{"plant", "fungus"}, Category: "Nature"},
		"earth":             {Emoji: "\U0001F30E", Name: "earth", Keywords: []string{"world", "globe", "planet"}, Category: "Nature"},

		// Objects & Symbols
		"100":               {Emoji: "\U0001F4AF", Name: "100", Keywords: []string{"score", "perfect"}, Category: "Symbols"},
		"check":             {Emoji: "\u2705", Name: "check", Keywords: []string{"yes", "done", "correct"}, Category: "Symbols"},
		"x":                 {Emoji: "\u274C", Name: "x", Keywords: []string{"no", "wrong", "cross"}, Category: "Symbols"},
		"question":          {Emoji: "\u2753", Name: "question", Keywords: []string{"what", "confused"}, Category: "Symbols"},
		"exclamation":       {Emoji: "\u2757", Name: "exclamation", Keywords: []string{"important", "alert"}, Category: "Symbols"},
		"warning":           {Emoji: "\u26A0\uFE0F", Name: "warning", Keywords: []string{"caution", "danger"}, Category: "Symbols"},
		"no_entry":          {Emoji: "\u26D4", Name: "no_entry", Keywords: []string{"stop", "forbidden"}, Category: "Symbols"},
		"recycle":           {Emoji: "\u267B\uFE0F", Name: "recycle", Keywords: []string{"green", "environment"}, Category: "Symbols"},
		"zzz":               {Emoji: "\U0001F4A4", Name: "zzz", Keywords: []string{"sleep", "tired"}, Category: "Symbols"},
		"boom":              {Emoji: "\U0001F4A5", Name: "boom", Keywords: []string{"explosion", "collision"}, Category: "Symbols"},
		"sparkles":          {Emoji: "\u2728", Name: "sparkles", Keywords: []string{"magic", "shine"}, Category: "Symbols"},
		"dizzy_symbol":      {Emoji: "\U0001F4AB", Name: "dizzy_symbol", Keywords: []string{"star", "spin"}, Category: "Symbols"},
		"sweat_drops":       {Emoji: "\U0001F4A6", Name: "sweat_drops", Keywords: []string{"water", "wet"}, Category: "Symbols"},
		"speech":            {Emoji: "\U0001F4AC", Name: "speech", Keywords: []string{"talk", "bubble"}, Category: "Symbols"},
		"thought":           {Emoji: "\U0001F4AD", Name: "thought", Keywords: []string{"think", "bubble"}, Category: "Symbols"},
		"phone":             {Emoji: "\U0001F4F1", Name: "phone", Keywords: []string{"mobile", "cell"}, Category: "Objects"},
		"computer":          {Emoji: "\U0001F4BB", Name: "computer", Keywords: []string{"laptop", "mac"}, Category: "Objects"},
		"keyboard":          {Emoji: "\u2328\uFE0F", Name: "keyboard", Keywords: []string{"type", "computer"}, Category: "Objects"},
		"printer":           {Emoji: "\U0001F5A8\uFE0F", Name: "printer", Keywords: []string{"computer", "paper"}, Category: "Objects"},
		"mouse_computer":    {Emoji: "\U0001F5B1\uFE0F", Name: "mouse_computer", Keywords: []string{"computer", "click"}, Category: "Objects"},
		"camera":            {Emoji: "\U0001F4F7", Name: "camera", Keywords: []string{"photo", "picture"}, Category: "Objects"},
		"video":             {Emoji: "\U0001F4F9", Name: "video", Keywords: []string{"movie", "film"}, Category: "Objects"},
		"tv":                {Emoji: "\U0001F4FA", Name: "tv", Keywords: []string{"television", "watch"}, Category: "Objects"},
		"radio":             {Emoji: "\U0001F4FB", Name: "radio", Keywords: []string{"music", "podcast"}, Category: "Objects"},
		"microphone":        {Emoji: "\U0001F3A4", Name: "microphone", Keywords: []string{"sing", "karaoke"}, Category: "Objects"},
		"headphones":        {Emoji: "\U0001F3A7", Name: "headphones", Keywords: []string{"music", "audio"}, Category: "Objects"},
		"guitar":            {Emoji: "\U0001F3B8", Name: "guitar", Keywords: []string{"music", "rock"}, Category: "Objects"},
		"piano":             {Emoji: "\U0001F3B9", Name: "piano", Keywords: []string{"music", "keys"}, Category: "Objects"},
		"drum":              {Emoji: "\U0001F941", Name: "drum", Keywords: []string{"music", "beat"}, Category: "Objects"},
		"book":              {Emoji: "\U0001F4D6", Name: "book", Keywords: []string{"read", "study"}, Category: "Objects"},
		"pencil":            {Emoji: "\u270F\uFE0F", Name: "pencil", Keywords: []string{"write", "draw"}, Category: "Objects"},
		"pen":               {Emoji: "\U0001F58A\uFE0F", Name: "pen", Keywords: []string{"write"}, Category: "Objects"},
		"paintbrush":        {Emoji: "\U0001F58C\uFE0F", Name: "paintbrush", Keywords: []string{"art", "draw"}, Category: "Objects"},
		"scissors":          {Emoji: "\u2702\uFE0F", Name: "scissors", Keywords: []string{"cut"}, Category: "Objects"},
		"paperclip":         {Emoji: "\U0001F4CE", Name: "paperclip", Keywords: []string{"attach"}, Category: "Objects"},
		"lock":              {Emoji: "\U0001F512", Name: "lock", Keywords: []string{"secure", "closed"}, Category: "Objects"},
		"unlock":            {Emoji: "\U0001F513", Name: "unlock", Keywords: []string{"open"}, Category: "Objects"},
		"key":               {Emoji: "\U0001F511", Name: "key", Keywords: []string{"open", "lock"}, Category: "Objects"},
		"hammer":            {Emoji: "\U0001F528", Name: "hammer", Keywords: []string{"tool", "build"}, Category: "Objects"},
		"wrench":            {Emoji: "\U0001F527", Name: "wrench", Keywords: []string{"tool", "fix"}, Category: "Objects"},
		"gear":              {Emoji: "\u2699\uFE0F", Name: "gear", Keywords: []string{"settings", "cog"}, Category: "Objects"},
		"bulb":              {Emoji: "\U0001F4A1", Name: "bulb", Keywords: []string{"idea", "light"}, Category: "Objects"},
		"flashlight":        {Emoji: "\U0001F526", Name: "flashlight", Keywords: []string{"light", "torch"}, Category: "Objects"},
		"battery":           {Emoji: "\U0001F50B", Name: "battery", Keywords: []string{"power", "energy"}, Category: "Objects"},
		"magnet":            {Emoji: "\U0001F9F2", Name: "magnet", Keywords: []string{"attract"}, Category: "Objects"},
		"bomb":              {Emoji: "\U0001F4A3", Name: "bomb", Keywords: []string{"explosive", "danger"}, Category: "Objects"},
		"gun":               {Emoji: "\U0001F52B", Name: "gun", Keywords: []string{"weapon", "water"}, Category: "Objects"},
		"pill":              {Emoji: "\U0001F48A", Name: "pill", Keywords: []string{"medicine", "drug"}, Category: "Objects"},
		"syringe":           {Emoji: "\U0001F489", Name: "syringe", Keywords: []string{"medicine", "vaccine"}, Category: "Objects"},
		"money":             {Emoji: "\U0001F4B0", Name: "money", Keywords: []string{"cash", "dollar"}, Category: "Objects"},
		"dollar":            {Emoji: "\U0001F4B5", Name: "dollar", Keywords: []string{"money", "cash"}, Category: "Objects"},
		"credit_card":       {Emoji: "\U0001F4B3", Name: "credit_card", Keywords: []string{"money", "pay"}, Category: "Objects"},
		"email":             {Emoji: "\U0001F4E7", Name: "email", Keywords: []string{"mail", "message"}, Category: "Objects"},
		"envelope":          {Emoji: "\u2709\uFE0F", Name: "envelope", Keywords: []string{"mail", "letter"}, Category: "Objects"},
		"package":           {Emoji: "\U0001F4E6", Name: "package", Keywords: []string{"box", "delivery"}, Category: "Objects"},
		"gift":              {Emoji: "\U0001F381", Name: "gift", Keywords: []string{"present", "birthday"}, Category: "Objects"},
		"trophy":            {Emoji: "\U0001F3C6", Name: "trophy", Keywords: []string{"win", "award"}, Category: "Objects"},
		"medal":             {Emoji: "\U0001F3C5", Name: "medal", Keywords: []string{"win", "sports"}, Category: "Objects"},
		"crown":             {Emoji: "\U0001F451", Name: "crown", Keywords: []string{"king", "queen", "royal"}, Category: "Objects"},
		"gem":               {Emoji: "\U0001F48E", Name: "gem", Keywords: []string{"diamond", "jewel"}, Category: "Objects"},
		"ring":              {Emoji: "\U0001F48D", Name: "ring", Keywords: []string{"wedding", "diamond"}, Category: "Objects"},
		"balloon":           {Emoji: "\U0001F388", Name: "balloon", Keywords: []string{"party", "celebration"}, Category: "Objects"},
		"confetti":          {Emoji: "\U0001F38A", Name: "confetti", Keywords: []string{"party", "celebration"}, Category: "Objects"},
		"tada":              {Emoji: "\U0001F389", Name: "tada", Keywords: []string{"party", "celebration", "hooray"}, Category: "Objects"},

		// Transport & Places
		"car":               {Emoji: "\U0001F697", Name: "car", Keywords: []string{"vehicle", "drive"}, Category: "Transport"},
		"taxi":              {Emoji: "\U0001F695", Name: "taxi", Keywords: []string{"car", "uber"}, Category: "Transport"},
		"bus":               {Emoji: "\U0001F68C", Name: "bus", Keywords: []string{"vehicle", "transport"}, Category: "Transport"},
		"train":             {Emoji: "\U0001F686", Name: "train", Keywords: []string{"vehicle", "transport"}, Category: "Transport"},
		"plane":             {Emoji: "\u2708\uFE0F", Name: "plane", Keywords: []string{"fly", "travel", "airplane"}, Category: "Transport"},
		"rocket":            {Emoji: "\U0001F680", Name: "rocket", Keywords: []string{"space", "launch"}, Category: "Transport"},
		"ship":              {Emoji: "\U0001F6A2", Name: "ship", Keywords: []string{"boat", "cruise"}, Category: "Transport"},
		"bicycle":           {Emoji: "\U0001F6B2", Name: "bicycle", Keywords: []string{"bike", "ride"}, Category: "Transport"},
		"motorcycle":        {Emoji: "\U0001F3CD\uFE0F", Name: "motorcycle", Keywords: []string{"bike", "ride"}, Category: "Transport"},
		"helicopter":        {Emoji: "\U0001F681", Name: "helicopter", Keywords: []string{"fly", "chopper"}, Category: "Transport"},
		"anchor":            {Emoji: "\u2693", Name: "anchor", Keywords: []string{"ship", "boat"}, Category: "Transport"},
		"fuel":              {Emoji: "\u26FD", Name: "fuel", Keywords: []string{"gas", "pump"}, Category: "Transport"},
		"traffic_light":     {Emoji: "\U0001F6A6", Name: "traffic_light", Keywords: []string{"stop", "go"}, Category: "Transport"},
		"house":             {Emoji: "\U0001F3E0", Name: "house", Keywords: []string{"home", "building"}, Category: "Places"},
		"office":            {Emoji: "\U0001F3E2", Name: "office", Keywords: []string{"building", "work"}, Category: "Places"},
		"hospital":          {Emoji: "\U0001F3E5", Name: "hospital", Keywords: []string{"health", "doctor"}, Category: "Places"},
		"school":            {Emoji: "\U0001F3EB", Name: "school", Keywords: []string{"education", "study"}, Category: "Places"},
		"bank":              {Emoji: "\U0001F3E6", Name: "bank", Keywords: []string{"money", "building"}, Category: "Places"},
		"hotel":             {Emoji: "\U0001F3E8", Name: "hotel", Keywords: []string{"travel", "sleep"}, Category: "Places"},
		"church":            {Emoji: "\u26EA", Name: "church", Keywords: []string{"religion", "building"}, Category: "Places"},
		"mosque":            {Emoji: "\U0001F54C", Name: "mosque", Keywords: []string{"religion", "building"}, Category: "Places"},
		"synagogue":         {Emoji: "\U0001F54D", Name: "synagogue", Keywords: []string{"religion", "building"}, Category: "Places"},
		"tent":              {Emoji: "\u26FA", Name: "tent", Keywords: []string{"camping", "outdoor"}, Category: "Places"},
		"stadium":           {Emoji: "\U0001F3DF\uFE0F", Name: "stadium", Keywords: []string{"sports", "arena"}, Category: "Places"},
		"mountain":          {Emoji: "\u26F0\uFE0F", Name: "mountain", Keywords: []string{"nature", "hike"}, Category: "Places"},
		"beach":             {Emoji: "\U0001F3D6\uFE0F", Name: "beach", Keywords: []string{"vacation", "summer"}, Category: "Places"},
		"island":            {Emoji: "\U0001F3DD\uFE0F", Name: "island", Keywords: []string{"vacation", "tropical"}, Category: "Places"},
		"camping":           {Emoji: "\U0001F3D5\uFE0F", Name: "camping", Keywords: []string{"outdoor", "tent"}, Category: "Places"},
		"statue_of_liberty": {Emoji: "\U0001F5FD", Name: "statue_of_liberty", Keywords: []string{"usa", "nyc"}, Category: "Places"},

		// Activities & Sports
		"soccer":            {Emoji: "\u26BD", Name: "soccer", Keywords: []string{"sports", "football"}, Category: "Activities"},
		"basketball":        {Emoji: "\U0001F3C0", Name: "basketball", Keywords: []string{"sports", "ball"}, Category: "Activities"},
		"football":          {Emoji: "\U0001F3C8", Name: "football", Keywords: []string{"sports", "american"}, Category: "Activities"},
		"baseball":          {Emoji: "\u26BE", Name: "baseball", Keywords: []string{"sports", "ball"}, Category: "Activities"},
		"tennis":            {Emoji: "\U0001F3BE", Name: "tennis", Keywords: []string{"sports", "ball"}, Category: "Activities"},
		"volleyball":        {Emoji: "\U0001F3D0", Name: "volleyball", Keywords: []string{"sports", "ball"}, Category: "Activities"},
		"golf":              {Emoji: "\u26F3", Name: "golf", Keywords: []string{"sports"}, Category: "Activities"},
		"bowling":           {Emoji: "\U0001F3B3", Name: "bowling", Keywords: []string{"sports"}, Category: "Activities"},
		"ping_pong":         {Emoji: "\U0001F3D3", Name: "ping_pong", Keywords: []string{"sports", "table tennis"}, Category: "Activities"},
		"badminton":         {Emoji: "\U0001F3F8", Name: "badminton", Keywords: []string{"sports"}, Category: "Activities"},
		"hockey":            {Emoji: "\U0001F3D2", Name: "hockey", Keywords: []string{"sports", "ice"}, Category: "Activities"},
		"ski":               {Emoji: "\U0001F3BF", Name: "ski", Keywords: []string{"sports", "winter"}, Category: "Activities"},
		"snowboard":         {Emoji: "\U0001F3C2", Name: "snowboard", Keywords: []string{"sports", "winter"}, Category: "Activities"},
		"swim":              {Emoji: "\U0001F3CA", Name: "swim", Keywords: []string{"sports", "water"}, Category: "Activities"},
		"surf":              {Emoji: "\U0001F3C4", Name: "surf", Keywords: []string{"sports", "water"}, Category: "Activities"},
		"running":           {Emoji: "\U0001F3C3", Name: "running", Keywords: []string{"sports", "exercise"}, Category: "Activities"},
		"biking":            {Emoji: "\U0001F6B4", Name: "biking", Keywords: []string{"sports", "cycling"}, Category: "Activities"},
		"weightlifting":     {Emoji: "\U0001F3CB\uFE0F", Name: "weightlifting", Keywords: []string{"sports", "gym"}, Category: "Activities"},
		"yoga":              {Emoji: "\U0001F9D8", Name: "yoga", Keywords: []string{"exercise", "meditation"}, Category: "Activities"},
		"dart":              {Emoji: "\U0001F3AF", Name: "dart", Keywords: []string{"target", "game"}, Category: "Activities"},
		"game":              {Emoji: "\U0001F3AE", Name: "game", Keywords: []string{"video", "play"}, Category: "Activities"},
		"joystick":          {Emoji: "\U0001F579\uFE0F", Name: "joystick", Keywords: []string{"game", "controller"}, Category: "Activities"},
		"slot_machine":      {Emoji: "\U0001F3B0", Name: "slot_machine", Keywords: []string{"casino", "gamble"}, Category: "Activities"},
		"puzzle":            {Emoji: "\U0001F9E9", Name: "puzzle", Keywords: []string{"game", "piece"}, Category: "Activities"},
		"chess":             {Emoji: "\u265F\uFE0F", Name: "chess", Keywords: []string{"game", "board"}, Category: "Activities"},
		"dice":              {Emoji: "\U0001F3B2", Name: "dice", Keywords: []string{"game", "random"}, Category: "Activities"},

		// Flags
		"flag_us":           {Emoji: "\U0001F1FA\U0001F1F8", Name: "flag_us", Keywords: []string{"usa", "america"}, Category: "Flags"},
		"flag_uk":           {Emoji: "\U0001F1EC\U0001F1E7", Name: "flag_uk", Keywords: []string{"britain", "england"}, Category: "Flags"},
		"flag_fr":           {Emoji: "\U0001F1EB\U0001F1F7", Name: "flag_fr", Keywords: []string{"france"}, Category: "Flags"},
		"flag_de":           {Emoji: "\U0001F1E9\U0001F1EA", Name: "flag_de", Keywords: []string{"germany"}, Category: "Flags"},
		"flag_it":           {Emoji: "\U0001F1EE\U0001F1F9", Name: "flag_it", Keywords: []string{"italy"}, Category: "Flags"},
		"flag_es":           {Emoji: "\U0001F1EA\U0001F1F8", Name: "flag_es", Keywords: []string{"spain"}, Category: "Flags"},
		"flag_jp":           {Emoji: "\U0001F1EF\U0001F1F5", Name: "flag_jp", Keywords: []string{"japan"}, Category: "Flags"},
		"flag_kr":           {Emoji: "\U0001F1F0\U0001F1F7", Name: "flag_kr", Keywords: []string{"korea", "south korea"}, Category: "Flags"},
		"flag_cn":           {Emoji: "\U0001F1E8\U0001F1F3", Name: "flag_cn", Keywords: []string{"china"}, Category: "Flags"},
		"flag_in":           {Emoji: "\U0001F1EE\U0001F1F3", Name: "flag_in", Keywords: []string{"india"}, Category: "Flags"},
		"flag_br":           {Emoji: "\U0001F1E7\U0001F1F7", Name: "flag_br", Keywords: []string{"brazil"}, Category: "Flags"},
		"flag_ru":           {Emoji: "\U0001F1F7\U0001F1FA", Name: "flag_ru", Keywords: []string{"russia"}, Category: "Flags"},
		"flag_ca":           {Emoji: "\U0001F1E8\U0001F1E6", Name: "flag_ca", Keywords: []string{"canada"}, Category: "Flags"},
		"flag_au":           {Emoji: "\U0001F1E6\U0001F1FA", Name: "flag_au", Keywords: []string{"australia"}, Category: "Flags"},
		"flag_mx":           {Emoji: "\U0001F1F2\U0001F1FD", Name: "flag_mx", Keywords: []string{"mexico"}, Category: "Flags"},
		"checkered_flag":    {Emoji: "\U0001F3C1", Name: "checkered_flag", Keywords: []string{"race", "finish"}, Category: "Flags"},
		"pirate_flag":       {Emoji: "\U0001F3F4\u200D\u2620\uFE0F", Name: "pirate_flag", Keywords: []string{"skull", "jolly roger"}, Category: "Flags"},
		"rainbow_flag":      {Emoji: "\U0001F3F3\uFE0F\u200D\U0001F308", Name: "rainbow_flag", Keywords: []string{"pride", "lgbtq"}, Category: "Flags"},
		"white_flag":        {Emoji: "\U0001F3F3\uFE0F", Name: "white_flag", Keywords: []string{"surrender", "peace"}, Category: "Flags"},
	}
}
