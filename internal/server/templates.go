package server

import "github.com/abjelosevic88/pocket-homelab-forum/internal/store"

// forumTemplate is a way to start a forum: its categories and first words.
type forumTemplate struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Line       string           `json:"line"`
	Emoji      string           `json:"emoji"`
	Visibility string           `json:"visibility"`
	JoinPolicy string           `json:"join_policy"`
	Categories []store.Category `json:"categories"`
	Rules      string           `json:"rules"`
	Welcome    string           `json:"welcome"`
}

func cat(name, emoji, color, about string, mods, qa bool) store.Category {
	c := store.Category{Name: name, Slug: store.Slugify(name, "-"), Emoji: emoji, Color: color, About: about, Starts: "all", QA: qa}
	if mods {
		c.Starts = "mods"
	}
	return c
}

const kindRules = `**Be kind.** Disagree with ideas, not with people.

**Stay on topic.** Start a new topic for a new subject — it keeps answers easy to find.

**Search first.** Someone may have asked already.

**Mark the answer.** When a reply solves your question, mark it as the solution.

**Flag, don't fight.** If something is wrong, flag it and a moderator will look.`

var forumTemplates = []forumTemplate{
	{ID: "community", Name: "Community", Line: "A place for people who share an interest: news, questions, ideas and chat.", Emoji: "💬", Visibility: "box", JoinPolicy: "open",
		Categories: []store.Category{
			cat("Announcements", "📣", "#c2410c", "News from the people who run this forum.", true, false),
			cat("General", "💬", "#4361c2", "Anything that does not fit somewhere else.", false, false),
			cat("Questions", "❓", "#0f766e", "Ask, and mark the reply that helped as the solution.", false, true),
			cat("Ideas & feedback", "💡", "#a16207", "What could be better, and what you would like to see.", false, false),
			cat("Off-topic", "☕", "#6b7280", "Chat about everything else.", false, false),
		}, Rules: kindRules, Welcome: "Welcome! Say hello in **General**, ask in **Questions**, and have a look at the rules first."},
	{ID: "support", Name: "Help & support", Line: "Questions that get answered, how-to guides, bug reports and wishes — for an app, a product or a service.", Emoji: "🛟", Visibility: "public", JoinPolicy: "open",
		Categories: []store.Category{
			cat("Announcements", "📣", "#c2410c", "New versions and news.", true, false),
			cat("Help", "🛟", "#0f766e", "Ask how to do something. Mark the reply that solved it.", false, true),
			cat("Guides", "📘", "#4361c2", "Step-by-step how-tos written by the community.", false, false),
			cat("Bug reports", "🐞", "#b42318", "Something does not work as it should: what you did, what happened, what you expected.", false, false),
			cat("Feature requests", "✨", "#7c3aed", "What you would like it to do next.", false, false),
		}, Rules: kindRules, Welcome: "Welcome! Search first — your question may already have an answer. Otherwise ask in **Help**."},
	{ID: "club", Name: "Club or team", Line: "For a sports club, a choir, a team: news, events with who is coming, results and gear.", Emoji: "⚽", Visibility: "private", JoinPolicy: "approval",
		Categories: []store.Category{
			cat("News", "📣", "#c2410c", "From the committee and the coaches.", true, false),
			cat("Events", "📅", "#2563eb", "Trainings, matches, meetings and nights out — say whether you are coming.", false, false),
			cat("Results", "🏆", "#a16207", "How it went.", false, false),
			cat("Kit & gear", "🎽", "#0f766e", "Buy, sell, swap and lend.", false, false),
			cat("Chat", "💬", "#6b7280", "Everything else.", false, false),
		}, Rules: kindRules, Welcome: "Welcome to the club! Events are in **Events** — tap *Going* so we know you are coming."},
	{ID: "family", Name: "Family", Line: "A calm place for the family: news, plans, recipes and memories, away from the group chat.", Emoji: "🏡", Visibility: "private", JoinPolicy: "invite",
		Categories: []store.Category{
			cat("News", "📰", "#c2410c", "What is happening.", false, false),
			cat("Plans", "📅", "#2563eb", "Visits, birthdays, holidays — with dates.", false, false),
			cat("Recipes", "🍲", "#15803d", "Grandma's and everyone else's.", false, false),
			cat("Memories", "📸", "#a83b6c", "Old photos and the stories behind them.", false, false),
			cat("Questions", "❓", "#0f766e", "Who knows how to…?", false, true),
		}, Welcome: "Welcome home. Plans go in **Plans**, old photos in **Memories**."},
	{ID: "team", Name: "Work team", Line: "Decisions, questions and ideas for a team — searchable later, unlike chat.", Emoji: "🧭", Visibility: "private", JoinPolicy: "invite",
		Categories: []store.Category{
			cat("Announcements", "📣", "#c2410c", "Decisions and news everyone should read.", true, false),
			cat("Questions", "❓", "#0f766e", "Ask the team. Mark the answer.", false, true),
			cat("Proposals", "📝", "#4361c2", "Ideas to discuss before they become work.", false, false),
			cat("How we work", "📘", "#7c3aed", "Agreed ways of doing things.", false, false),
			cat("Watercooler", "☕", "#6b7280", "Everything else.", false, false),
		}, Rules: kindRules},
	{ID: "bookclub", Name: "Book club", Line: "What you are reading, what to read next and when you meet.", Emoji: "📚", Visibility: "private", JoinPolicy: "approval",
		Categories: []store.Category{
			cat("Now reading", "📖", "#7c3aed", "One topic per book, with spoiler warnings.", false, false),
			cat("Suggestions", "💡", "#a16207", "Books for the next round — vote in the polls.", false, false),
			cat("Meetings", "📅", "#2563eb", "When and where we meet.", false, false),
			cat("Off-topic", "☕", "#6b7280", "Everything else.", false, false),
		}, Welcome: "Welcome to the book club! Suggest a book in **Suggestions**."},
	{ID: "blank", Name: "Start empty", Line: "One category; add your own.", Emoji: "💬", Visibility: "private", JoinPolicy: "invite",
		Categories: []store.Category{cat("General", "💬", "#4361c2", "", false, false)}},
}

func templateByID(id string) forumTemplate {
	for _, t := range forumTemplates {
		if t.ID == id {
			return t
		}
	}
	return forumTemplates[len(forumTemplates)-1]
}
